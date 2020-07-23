package executor

import (
	"github.com/sirupsen/logrus"
	"sync"
)

type Executor struct {
	runningLocker sync.RWMutex
	running       bool

	downloadChannel chan *Command
	dManager        *DownloaderManager

	processChannel chan *Command
	processor      *Processor

	once sync.Once
}

func NewExecutor() *Executor {
	e := &Executor{
		downloadChannel: make(chan *Command),
		processChannel:  make(chan *Command),
	}
	e.dManager = newDownloaderManager(e.downloadChannel, e.processChannel)
	e.processor = newProcessor(e.processChannel, e.downloadChannel)
	e.running = true
	return e
}

func (e *Executor) AcceptTask(task AbsTask) bool {
	e.runningLocker.RLock()
	defer e.runningLocker.RUnlock()

	if e.running {
		cmds := task.GetCommands()
		for _, cmd := range cmds {
			e.downloadChannel <- cmd

			logrus.WithFields(logrus.Fields{
				"TargetURL": cmd.ctx.Request.Url,
				"Task":      cmd.ctx.Task.GetTaskName(),
			}).Debug("接受 Task")
		}
		return true
	} else {
		return false
	}
}

func (e *Executor) WaitAndStop() {
	e.once.Do(func() {
		logrus.Debug("开始关闭 Executor")

		go e.dropDataInDownloadChannel()
		go e.dropDataInProcessChannel()
		e.dManager.WaitAndStop()
		e.processor.WaitAndStop()
		close(e.downloadChannel)
		close(e.processChannel)

		logrus.Info("已关闭 Executor")
	})
}

func (e *Executor) dropDataInDownloadChannel() {
	cnt := 0
	for cmd := range e.downloadChannel {
		logrus.WithFields(logrus.Fields{
			"Task": cmd.ctx.Task.GetTaskName(),
			"URL":  cmd.ctx.Request.Url.String(),
		}).Debug("抛弃 Download Command")
		cnt++
	}
	logrus.WithField("数量", cnt).Info("抛弃 DownloaderChannel 完成, Channel 以及关闭")
}

func (e *Executor) dropDataInProcessChannel() {
	cnt := 0
	for cmd := range e.downloadChannel {
		logrus.WithFields(logrus.Fields{
			"Task": cmd.ctx.Task.GetTaskName(),
			"URL":  cmd.ctx.Request.Url.String(),
		}).Debug("抛弃 Process Command")
		cnt++
	}
	logrus.WithField("数量", cnt).Info("抛弃 ProcessChannel 完成, Channel 以及关闭")
}
