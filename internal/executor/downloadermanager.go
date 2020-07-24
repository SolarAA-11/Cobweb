package executor

import (
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
			downloader.Release()

			if cmd.ResponseLegal() {
				if !downloader.ResetErrCnt(d.maxDownloadErrCnt) {
					d.newReplaceDownloader(downloader)
				}
				d.cmdOutChan <- cmd

				logEntry.WithFields(logrus.Fields{
					"Proxy": downloader.GetProxyUsed(),
				}).WithFields(cmd.ctx.LogrusFields()).Debug("完成请求 发送到 cmdOutChan")
			} else {
				if !downloader.IncreaseErrCnt(d.maxDownloadErrCnt) {
					d.newReplaceDownloader(downloader)
				}
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

func (d *DownloaderManager) newReplaceDownloader(downloader *Downloader) {
	d.sliceLocker.Lock()
	defer d.sliceLocker.Unlock()

	dIndex := 0
	for dIndex < len(d.downloaderSlice) && d.downloaderSlice[dIndex] != downloader {
		dIndex++
	}
	if dIndex >= len(d.downloaderSlice) {
		logrus.WithFields(logrus.Fields{}).Fatal("Index 越界")
	}

	newDownloader := d.factory.NewDownloader(d.downloaderSlice, d.maxDownloaderConcurrentReqCnt, d.downloaderCnt*2)
	if newDownloader == nil {
		logrus.WithFields(logrus.Fields{}).Fatal("新建 Downloader 失败")
	}

	d.downloaderSlice[dIndex] = newDownloader

	logrus.WithFields(logrus.Fields{
		"OldProxy": downloader.GetProxyUsed(),
		"NewProxy": downloader.GetProxyUsed(),
	}).Info("更换 Downloader")
}

func (d *DownloaderManager) getProperDownloader(cmd *Command) *Downloader {
	d.sliceLocker.Lock()
	defer d.sliceLocker.Unlock()

	var downloader *Downloader = nil
	for _, d2 := range d.downloaderSlice {
		if d2.TryAcquire() {
			lastReqTime := d2.GetLastReqTime(cmd.ctx.Request)
			requireTime := lastReqTime.Add(d.reqTimeInterval)
			if requireTime.Before(time.Now()) {
				downloader = d2
				break
			} else {
				d2.Release()
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
	logrus.Info("已经关闭 DownloaderManager")
}
