package cobweb

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Executor is a main part of cobweb
// it accepts rule and creates task according to that.
//
//
type Executor struct {
	dManager  absDownloaderManager
	parser    *parser
	pipeliner *pipeliner

	downloadCMDChannel  chan *command
	parseCMDChannel     chan *command
	pipeItemInfoChannel chan *itemInfo

	stopOnce sync.Once
	stopWg   sync.WaitGroup

	runningLocker sync.Mutex
	running       bool
}

func NewNoProxyDefaultExecutor() *Executor {
	return NewExecutor(
		&NoProxyFastHTTPDownloaderFactory{},
		1,
		20,
		10,
		time.Second*2,
	)
}

func NewDefaultExecutor() *Executor {
	return NewExecutor(
		&ProxyFastHTTPDownloaderFactory{},
		100,
		2,
		10,
		time.Second*3,
	)
}

func NewExecutor(
	dFactory downloaderFactory,
	downloaderCnt int,
	downloaderConcurrentLimit int,
	downloaderErrCntLimit int,
	downloaderReqHostInterval time.Duration,
) *Executor {
	e := &Executor{
		downloadCMDChannel:  make(chan *command, 500),
		parseCMDChannel:     make(chan *command, 500),
		pipeItemInfoChannel: make(chan *itemInfo, 500),
	}

	e.dManager = newDownloaderManager(
		dFactory,
		downloaderCnt,
		downloaderConcurrentLimit,
		downloaderErrCntLimit,
		downloaderReqHostInterval,
		e.downloadCMDChannel,
		e.parseCMDChannel,
	)

	e.parser = newParser(e.parseCMDChannel, e.downloadCMDChannel, e.pipeItemInfoChannel)
	e.pipeliner = newPipeliner(e.pipeItemInfoChannel)
	e.running = true

	return e
}

func NewExecutorWithSimpleDownloaderManager() *Executor {
	e := &Executor{
		downloadCMDChannel:  make(chan *command, 100),
		parseCMDChannel:     make(chan *command, 100),
		pipeItemInfoChannel: make(chan *itemInfo, 100),
	}

	e.dManager = newSimpleDownloaderManager(
		e.downloadCMDChannel,
		e.parseCMDChannel,
		200,
	)

	e.parser = newParser(e.parseCMDChannel, e.downloadCMDChannel, e.pipeItemInfoChannel)
	e.pipeliner = newPipeliner(e.pipeItemInfoChannel)
	e.running = true

	return e
}

// accept rule and create task for it
// if executor is not running return nil
func (e *Executor) AcceptRule(rule BaseRule) *Task {
	e.runningLocker.Lock()
	defer e.runningLocker.Unlock()
	if !e.running {
		return nil
	}

	task := newTaskFromRule(rule)
	if task == nil {
		return nil
	}

	initCMDs := task.initCommands()
	for _, cmd := range initCMDs {
		e.downloadCMDChannel <- cmd
	}
	logrus.WithFields(logrus.Fields{
		"TaskName":     task.Name(),
		"InitCMDCount": len(initCMDs),
	}).Info("Cobweb accept rule.")
	return task
}

func (e *Executor) Stop() {
	e.runningLocker.Lock()
	defer e.runningLocker.Unlock()
	if !e.running {
		logrus.Info("Cobweb executor is not running.")
		return
	}

	e.stopOnce.Do(func() {
		logrus.Info("Cobweb executor stopping...")

		go e.dropCommandUntilClosed(e.downloadCMDChannel, "DownloadCMDChannel")
		go e.dropCommandUntilClosed(e.parseCMDChannel, "ParseCMDChannel")
		go e.dropItemInfoUntilChannelClosed(e.pipeItemInfoChannel, "PipeItemInfoChannel")

		e.dManager.stop()
		e.parser.stop()
		e.pipeliner.stop()

		close(e.downloadCMDChannel)
		close(e.parseCMDChannel)
		close(e.pipeItemInfoChannel)
	})

	e.stopWg.Wait()
	e.running = false
	logrus.Info("Cobweb executor stopped.")
}

// drop all data in channel until closed
func (e *Executor) dropCommandUntilClosed(ch chan *command, chKind string) {
	e.stopWg.Add(1)
	defer e.stopWg.Done()

	droppedDataCount := 0
	for _ = range ch {
		droppedDataCount++
	}

	logrus.WithFields(logrus.Fields{
		"DroppedDataCnt": droppedDataCount,
		"ChannelKind":    chKind,
	}).Info("finish drop channel data, channel has been closed")
}

func (e *Executor) dropItemInfoUntilChannelClosed(ch chan *itemInfo, chKind string) {
	e.stopWg.Add(1)
	defer e.stopWg.Done()

	droppedDataCount := 0
	for _ = range ch {
		droppedDataCount++
	}

	logrus.WithFields(logrus.Fields{
		"DroppedDataCnt": droppedDataCount,
		"ChannelKind":    chKind,
	}).Info("finish drop channel data, channel has been closed")
}
