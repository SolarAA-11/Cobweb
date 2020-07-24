package executor

import (
	"github.com/SolarDomo/Cobweb/internal/proxypool/storage"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type DownloaderManager struct {
	sliceLocker                   sync.Mutex
	downloaderCnt                 int
	maxDownloaderConcurrentReqCnt int
	maxDownloadErrCnt             int
	downloaderSlice               []*Downloader
	factory                       AbsDownloaderFactory

	reqTimeInterval time.Duration

	cmdInChan  chan *Command
	cmdOutChan chan *Command

	once      sync.Once
	wg        sync.WaitGroup
	closeChan chan struct{}
}

func newDownloaderManager(
	inChan chan *Command,
	outChan chan *Command,
	downloaderCnt int,
	maxDownloaderConcurrentReqCnt int,
	maxDownloadErrCnt int,
	factory AbsDownloaderFactory,
	reqTimeInterval time.Duration,
) *DownloaderManager {
	d := &DownloaderManager{
		downloaderCnt:                 downloaderCnt,
		maxDownloaderConcurrentReqCnt: maxDownloaderConcurrentReqCnt,
		maxDownloadErrCnt:             maxDownloadErrCnt,
		downloaderSlice:               make([]*Downloader, 0, downloaderCnt),
		factory:                       factory,
		reqTimeInterval:               reqTimeInterval,
		cmdInChan:                     inChan,
		cmdOutChan:                    outChan,
		closeChan:                     make(chan struct{}),
	}

	for i := 0; i < d.downloaderCnt; i++ {
		newDownloader := d.factory.NewDownloader(d.downloaderSlice, d.maxDownloaderConcurrentReqCnt, d.downloaderCnt*2)
		if newDownloader == nil {
			logrus.Fatal("生成 Downloader 失败")
		}
		d.downloaderSlice = append(d.downloaderSlice, newDownloader)

		logrus.WithFields(logrus.Fields{
			"Proxy": newDownloader.GetProxyUsed(),
		}).Info("添加新 Downloader")
	}

	for i := 0; i < d.maxDownloaderConcurrentReqCnt*d.downloaderCnt; i++ {
		go d.workRoutine(i)
	}

	return d
}

func (d *DownloaderManager) workRoutine(routineID int) {
	logEntry := logrus.WithFields(logrus.Fields{
		"ID": routineID,
	})
	logEntry.Debug("启动 DownloaderManager 协程")
	d.wg.Add(1)

	loop := true
	for loop {
		select {
		case cmd, ok := <-d.cmdInChan:
			if !ok {
				loop = false
				break
			}

			downloader := d.getProperDownloader(cmd)
			if downloader == nil {
				go func() {
					logEntry.WithFields(logrus.Fields{
						"等待时间": d.reqTimeInterval,
					}).WithFields(cmd.ctx.LogrusFields()).Debug("暂无 Downloader 能够处理, 等待一段时间后返回通道")
					time.Sleep(d.reqTimeInterval + 1*time.Second)
					d.cmdInChan <- cmd
				}()
				continue
			}

			downloader.Do(cmd)
			downloader.Release(cmd.ctx.Request)

			if cmd.ResponseLegal() {
				if !downloader.ResetErrCnt(d.maxDownloadErrCnt) {
					d.replaceOldDownloader(downloader)
				}
				d.cmdOutChan <- cmd

				logEntry.WithFields(logrus.Fields{
					"Proxy": downloader.GetProxyUsed(),
				}).WithFields(cmd.ctx.LogrusFields()).Debug("完成请求 发送到 cmdOutChan")
			} else {
				if !downloader.IncreaseErrCnt(d.maxDownloadErrCnt) {
					d.replaceOldDownloader(downloader)
				}

				cmd.ctx.BaseTask.IncreaseErrCnt()
				d.cmdInChan <- cmd

				logEntry.WithFields(logrus.Fields{
					"Proxy": downloader.GetProxyUsed(),
				}).WithFields(cmd.ctx.LogrusFields()).Debug("请求失败 重新发送到 cmdInChan")
			}

		case <-d.closeChan:
			loop = false
		}
	}

	d.wg.Done()
	logEntry.Debug("关闭 DownloaderManager 协程")
}

func (d *DownloaderManager) replaceOldDownloader(old *Downloader) {
	d.sliceLocker.Lock()
	defer d.sliceLocker.Unlock()

	dIndex := 0
	for dIndex < len(d.downloaderSlice) && d.downloaderSlice[dIndex] != old {
		dIndex++
	}
	if dIndex >= len(d.downloaderSlice) {
		// 越界表示当前使用 downloader 在之前处理 Command 的时候就已经被踢出
		// 并使用了新的 Downloader 替换
		//logrus.WithFields(logrus.Fields{
		//	"Proxy": old.GetProxyUsed(),
		//}).Fatal("Index 越界")
		return
	}

	newDownloader := d.factory.NewDownloader(d.downloaderSlice, d.maxDownloaderConcurrentReqCnt, d.downloaderCnt*2)
	if newDownloader == nil {
		logrus.WithFields(logrus.Fields{}).Fatal("新建 Downloader 失败")
	}

	// 使用 dIndex 替换尽管速度稍快
	// 但是如果 dIndex 为 0, 则会一直优先使用第一个
	// 而通过之前的运行 后面可能更加可靠的 Downloader 无法得到有效的使用
	// d.downloaderSlice[dIndex] = newDownloader
	//
	// 由于获取 Downloader 的方法为顺序迭代 d.downloaderSlice
	// 则将失效的 Downloader 从 slice 中移除
	// 新的 Downloader 加入 slice 的尾部
	// 则跟可靠的 Downloader 靠前, 优先使用
	d.downloaderSlice = append(d.downloaderSlice[:dIndex], d.downloaderSlice[dIndex+1:]...)
	d.downloaderSlice = append(d.downloaderSlice, newDownloader)

	// 对 Old Downloader 的 Proxy 进行降权
	err := storage.Singleton().DeactivateProxy(old.GetProxyUsed())
	if err != nil {
		logrus.WithField("Proxy", &old).Error("代理降权失败")
	}

	logrus.WithFields(logrus.Fields{
		"OldProxy": old.GetProxyUsed(),
		"NewProxy": newDownloader.GetProxyUsed(),
	}).Info("更换 Downloader")
}

func (d *DownloaderManager) getProperDownloader(cmd *Command) *Downloader {
	d.sliceLocker.Lock()
	defer d.sliceLocker.Unlock()

	var downloader *Downloader = nil
	for _, d2 := range d.downloaderSlice {
		if d2.TryAcquire(cmd.ctx.Request) {
			lastReqTime := d2.GetLastReqTime(cmd.ctx.Request)
			requireTime := lastReqTime.Add(d.reqTimeInterval)
			if requireTime.Before(time.Now()) {
				downloader = d2
				break
			} else {
				d2.Release(cmd.ctx.Request)
			}
		}
	}

	return downloader
}

func (d *DownloaderManager) WaitAndStop() {
	logrus.Debug("开始关闭 DownloaderManager")
	d.once.Do(func() {
		close(d.closeChan)
	})
	d.wg.Wait()
	logrus.Info("已关闭 DownloaderManager")
}
