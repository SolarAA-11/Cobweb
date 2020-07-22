package cmddownloader

import (
	"container/list"
	"github.com/SolarDomo/Cobweb/internal/executor/command"
	"github.com/SolarDomo/Cobweb/internal/proxypool/storage"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

// 管理若干个 AbsCMDDownloader 的实例
// 使用 Fan Out 模式将 Command 分发到不同的协程 (fanOutChannel)
// 每个协程随机使用一个 Downloader 实例进行下载
// Downloader 下载后将结果添加到 Command 中
// 使用 Fan Out 模式, 将所有协程中处理完毕的 Command 集中到单独 Channel 中, 交给下一个流程处理 (fanInChannel)
type DownloaderManager struct {
	downloaderPool *sync.Pool

	timeout time.Duration

	downloader2Once       sync.Map
	proxyUsedListRWLocker sync.RWMutex
	proxyUsedList         *list.List

	runningRWLocker sync.RWMutex
	running         bool

	fanOutChannel chan command.AbsCommand
	fanInChannel  chan command.AbsCommand

	routineCloseWG sync.WaitGroup

	downloaderCnt           int
	routineCntPerDownloader int
	maxDownloaderErrCnt     int

	downloaderFactory AbsDownloaderFactory

	closeSyncOnce sync.Once
}

func NewDownloaderManager(
	timeout time.Duration,
	downloaderCnt int,
	routineCntPerDownloader int,
	maxDownloaderErrCnt int,
	downloaderFactory AbsDownloaderFactory,
) (*DownloaderManager, <-chan command.AbsCommand) {
	manager := &DownloaderManager{
		downloaderPool: &sync.Pool{},

		timeout: timeout,

		downloader2Once:       sync.Map{},
		proxyUsedListRWLocker: sync.RWMutex{},
		proxyUsedList:         list.New(),

		runningRWLocker: sync.RWMutex{},
		running:         true,

		fanOutChannel: make(chan command.AbsCommand),
		fanInChannel:  make(chan command.AbsCommand),

		routineCloseWG: sync.WaitGroup{},

		downloaderCnt:           downloaderCnt,
		routineCntPerDownloader: routineCntPerDownloader,
		maxDownloaderErrCnt:     maxDownloaderErrCnt,

		downloaderFactory: downloaderFactory,

		closeSyncOnce: sync.Once{},
	}

	// 启动的 worker 协程数量等于 downloaderCnt * routineCntPerDownloader
	for i := 0; i < manager.downloaderCnt*manager.routineCntPerDownloader; i++ {
		go manager.workerRoutine(i)
	}

	return manager, manager.fanInChannel
}

// 接受一个 Command, 将其发送到 FanOut Channel 上
// 如果接受, 返回 true; 如果拒绝, 返回 false
func (this *DownloaderManager) AcceptCommand(cmd command.AbsCommand) bool {
	this.runningRWLocker.RLock()
	defer this.runningRWLocker.RUnlock()
	if this.running {
		this.fanOutChannel <- cmd
		logrus.WithField("TargetURL", cmd.GetTargetURL()).Debug("DownloaderManager 接受 Command")
		return true
	} else {
		return false
	}
}

// 等待 Manager 中所有 Downloader 完成当前请求的内容
// 所有请求完毕后关闭 Manager
func (this *DownloaderManager) WaitAndStop() {
	this.closeSyncOnce.Do(func() {
		// 关闭 FanOutChannel
		this.runningRWLocker.Lock()
		this.running = false
		close(this.fanOutChannel)
		this.runningRWLocker.Unlock()

		//  等待全部 Routine 结束
		this.routineCloseWG.Wait()

		// 关闭 fanInChannel
		this.runningRWLocker.Lock()
		close(this.fanInChannel)
		this.runningRWLocker.Unlock()
	})

}

// Fan Out 的协程
func (this *DownloaderManager) workerRoutine(routineID int) {
	logEntry := logrus.WithField("RoutineID", routineID)
	logEntry.Debug("DownloaderManager 启动新 Worker 协程")

	this.routineCloseWG.Add(1)
	defer this.routineCloseWG.Done()

	for cmd := range this.fanOutChannel {
		downloader := this.getDownloaderFromPool()
		if downloader == nil {
			this.fanOutChannel <- cmd
			logrus.Error("Manager Worker Routine 获取 Downloader 失败")
			break
		}

		code, body, err := downloader.GetTimeout(cmd.GetTargetURL(), this.timeout)
		cmd.SetDownloadResult(code, body, err)
		this.fanInChannel <- cmd

		logEntry.WithFields(logrus.Fields{
			"TargetURL": cmd.GetTargetURL(),
			"Proxy":     downloader.GetProxyUsed(),
		}).Debug("DownloaderManager Worker 完成一次请求")

		// 如果 Downloader 的 ErrCnt 大于 maxDownloaderErrCnt
		// 则不将其加入到 Downloader Pool 中
		if err != nil || code != 200 {
			errCnt := downloader.IncreaseErrCnt()
			if errCnt >= this.maxDownloaderErrCnt {
				this.removeProxyUsed(downloader)
				continue
			}
		} else {
			if downloader.ResetErrCnt(this.maxDownloaderErrCnt) == false {
				this.removeProxyUsed(downloader)
				continue
			}
		}

		// Downloader ErrCnt 尚未达到最大值 将其归还到 Pool
		this.downloaderPool.Put(downloader)
	}

	logEntry.Debug("关闭 DownloaderManager Worker 协程")
}

// 从 ProxyList 中删除 downloader 对应的 Proxy
func (this *DownloaderManager) removeProxyUsed(downloader AbsCMDDownloader) {
	onceVal, ok := this.downloader2Once.Load(downloader)
	if !ok {
		return
	}

	once, ok := onceVal.(*sync.Once)
	if !ok {
		return
	}

	once.Do(func() {

		proxy := downloader.GetProxyUsed()

		this.proxyUsedListRWLocker.Lock()
		for e := this.proxyUsedList.Front(); e != nil; e = e.Next() {
			if e.Value == proxy {
				this.proxyUsedList.Remove(e)
				this.downloader2Once.Delete(downloader)
			}
		}
		this.proxyUsedListRWLocker.Unlock()

		// storage 中降权
		err := storage.Singleton().DeactivateProxy(proxy)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Proxy":      proxy,
				"Downloader": downloader.GetDownloaderName(),
			}).Error("Proxy 降权失败")
		}

		logrus.WithFields(logrus.Fields{
			"Proxy":      proxy,
			"Downloader": downloader.GetDownloaderName(),
		}).Info("DownloaderManager 删除 Downloader")
	})
}

// 从 Downloader Pool 中获取一个 Downloader
// 如果 Pool 为空, 则添加新的 Downloader 到 Pool 中
// 添加规则为
// 1. 如果 Downloader 使用 Proxy, 则 Proxy 必须在 Manager 中唯一
// 2. 根据参数 routineCntPerDownloader 在 Pool 中添加 Downloader
// 3. 添加到 Pool 中的 Downloader, 必须为某实例的指针
func (this *DownloaderManager) getDownloaderFromPool() AbsCMDDownloader {
	downloaderVal := this.downloaderPool.Get()
	if downloaderVal == nil {
		this.appendNewDownloaderToPool()
		downloaderVal = this.downloaderPool.Get()
		if downloaderVal == nil {
			logrus.Error("Downloader Manager 从 Pool 中获取协程失败")
			return nil
		}
	}

	downloader, ok := downloaderVal.(AbsCMDDownloader)
	if !ok {
		logrus.WithFields(logrus.Fields{
			"DownloaderVal": downloaderVal,
		}).Error("Downloader Manager 将 DownloaderVal 转换为 Downloader 失败")
		return nil
	}

	if downloader.CheckErrCnt(this.maxDownloaderErrCnt) == false {
		// 当前 ErrCnt 超过最大限制
		return this.getDownloaderFromPool()
	} else {
		return downloader
	}
}

// 向 Downloader Pool 中添加新的 Downloader
func (this *DownloaderManager) appendNewDownloaderToPool() {
	this.proxyUsedListRWLocker.RLock()
	newDownloader := this.downloaderFactory.NewDownloader(this.proxyUsedList, this.downloaderCnt)
	this.proxyUsedListRWLocker.RUnlock()

	if newDownloader == nil {
		logrus.WithFields(logrus.Fields{}).Error("Downloader Factory 建立新的 Downloader 失败")
		return
	}

	// 由于管理器中 每个 Client 对应若干个协程 routineCntPerDownloader
	// 所以需要向 downloaderPool 中添加 routineCntPerDownloader 个 newDownloader
	for i := 0; i < this.routineCntPerDownloader; i++ {
		this.downloaderPool.Put(newDownloader)
	}

	// 向 proxyUsedList 中添加新的 Proxy
	proxyUsed := newDownloader.GetProxyUsed()
	if proxyUsed != nil {
		this.proxyUsedListRWLocker.Lock()
		this.proxyUsedList.PushBack(newDownloader.GetProxyUsed())
		this.proxyUsedListRWLocker.Unlock()
	}

	// downloader2Once 添加新的 downloader 到 sync.Once
	this.downloader2Once.Store(newDownloader, &sync.Once{})

	logrus.WithFields(logrus.Fields{
		"Proxy":      proxyUsed,
		"Downloader": newDownloader.GetDownloaderName(),
	}).Info("DownloaderManager 添加新的 Downloader")
}
