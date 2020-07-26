package executor

import (
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type Executor struct {
	runningLocker sync.RWMutex
	running       bool

	downloadChan chan *Command
	dManager     *downloaderManager

	processChan chan *Command
	processor   *processor

	once sync.Once
	wg   sync.WaitGroup
}

func NewDefaultNoProxyExecutor() *Executor {
	return NewExecutor(
		&noProxyDownloaderFactory{},
		1,
		20,
		10,
		time.Second*3,
	)
}

func NewDefaultExecutor() *Executor {
	return NewExecutor(
		&proxyDownloaderFactory{},
		50,
		5,
		10,
		time.Second*3,
	)
}

func NewExecutor(
	dFactory downloaderFactory,
	downloaderCnt int,
	maxConcurrentReq int,
	maxErrCnt int,
	reqTimeInterval time.Duration,
) *Executor {
	if reqTimeInterval >= time.Second {
		reqTimeInterval -= time.Second

	}

	downloadChan := make(chan *Command)
	processChan := make(chan *Command)
	dm := newDownloaderManager(
		dFactory,
		downloaderCnt,
		maxConcurrentReq,
		maxErrCnt,
		reqTimeInterval,
		downloadChan,
		processChan,
	)
	p := newProcessor(processChan, downloadChan)

	e := &Executor{
		downloadChan: downloadChan,
		dManager:     dm,
		processChan:  processChan,
		processor:    p,
		running:      true,
	}
	return e
}

func (e *Executor) AcceptRule(rule BaseRule) *Task {
	e.runningLocker.RLock()
	defer e.runningLocker.RUnlock()
	if e.running {
		task := newTask(rule)
		if task == nil {
			return nil
		}

		logrus.WithFields(logrus.Fields{
			"TaskName": task.Name(),
		}).Info("Accept Rule")

		cmds := task.extractCommands()
		for _, cmd := range cmds {
			e.downloadChan <- cmd
		}
		return task
	} else {
		return nil
	}
}

func (e *Executor) Stop() {
	e.runningLocker.Lock()
	defer e.runningLocker.Unlock()
	e.once.Do(func() {
		logrus.Info("Executor stopping...")
		e.running = false
		go e.dropCMDChan(e.downloadChan, "Download Command Channel")
		go e.dropCMDChan(e.processChan, "Process Command Channel")
		e.dManager.Stop()
		e.processor.Stop()
		close(e.downloadChan)
		close(e.processChan)
	})
	e.wg.Wait()
	logrus.Info("Executor stopped.")
}

func (e *Executor) dropCMDChan(ch <-chan *Command, chName string) {
	e.wg.Add(1)
	defer e.wg.Done()
	for cmd := range ch {
		logrus.WithFields(cmd.ctx.LogrusFields()).WithField("ChanName", chName).Debug("Drop Command")
	}
	logrus.WithField("ChanName", chName).Info("Drop Channel Finished, Channel Closed")
}
