package executor

import (
	"errors"
	"github.com/sirupsen/logrus"
	"reflect"
	"sync"
	"time"
)

type Executor struct {
	runningLocker sync.RWMutex
	running       bool

	downloadChannel chan *Command
	dManager        *DownloaderManager

	processChannel chan *Command
	processor      *Processor

	once sync.Once
	wg   sync.WaitGroup
}

func NewDefaultNoProxyExecutor() *Executor {
	return NewExecutor(
		1,
		30,
		10,
		&NoProxyDownloaderFactory{},
		time.Second*2,
	)
}

func NewDefaultExecutor() *Executor {
	return NewExecutor(
		10,
		5,
		10,
		&DownloaderFactory{},
		time.Second*3,
	)
}

func NewExecutor(
	downloaderCnt int,
	maxDownloaderConcurrentReqCnt int,
	maxDownloadErrCnt int,
	factory AbsDownloaderFactory,
	reqTimeInterval time.Duration,
) *Executor {
	e := &Executor{
		downloadChannel: make(chan *Command),
		processChannel:  make(chan *Command),
	}
	e.dManager = newDownloaderManager(
		e.downloadChannel,
		e.processChannel,
		downloaderCnt,
		maxDownloaderConcurrentReqCnt,
		maxDownloadErrCnt,
		factory,
		reqTimeInterval,
	)
	e.processor = newProcessor(e.processChannel, e.downloadChannel)
	e.running = true
	return e
}

func (e *Executor) AcceptTask(task AbsTask) bool {
	e.runningLocker.RLock()
	defer e.runningLocker.RUnlock()

	if e.running {
		cmds := task.GetCommands()
		logrus.WithFields(logrus.Fields{
			"BaseTask":   task.GetTaskName(),
			"InitCMDCnt": len(cmds),
		}).Info("Executor 接受新 BaseTask")
		for _, cmd := range cmds {
			logrus.WithFields(cmd.ctx.LogrusFields()).Debug("接受 BaseTask Init Command")
			e.downloadChannel <- cmd
		}
		return true
	} else {
		return false
	}
}

func (e *Executor) AcceptTaskV2(task AbsTask) (bool, error) {
	valOfTask := reflect.ValueOf(task)
	if valOfTask.Kind() != reflect.Ptr {
		return false, errors.New("task 必须为某结构体指针")
	}

	valOfTask = valOfTask.Elem()
	if valOfTask.Kind() != reflect.Struct {
		return false, errors.New("")
	}

	// todo

	return false, nil
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
	})
	e.wg.Wait()
	logrus.Info("已关闭 Executor")
}

func (e *Executor) dropDataInDownloadChannel() {
	e.wg.Add(1)
	defer e.wg.Done()

	cnt := 0
	for cmd := range e.downloadChannel {
		logrus.WithFields(cmd.ctx.LogrusFields()).Debug("抛弃 Download Command")
		cnt++
	}
	logrus.WithField("数量", cnt).Info("Drop DownloaderChannel Data 完成, Channel 已经关闭")
}

func (e *Executor) dropDataInProcessChannel() {
	e.wg.Add(1)
	defer e.wg.Done()

	cnt := 0
	for cmd := range e.processChannel {
		logrus.WithFields(cmd.ctx.LogrusFields()).Debug("抛弃 Process Command")
		cnt++
	}
	logrus.WithField("数量", cnt).Info("Drop ProcessChannel Data 完成, Channel 已经关闭")
}
