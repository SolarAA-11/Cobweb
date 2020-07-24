package executor

import (
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"strings"
	"sync"
	"time"
)

type OnResponseCallback func(ctx *Context)

type AbsTask interface {
	GetTaskName() string
	GetCommands() []*Command
	GetCompleteChan() <-chan struct{}
	CommandComplete(command *Command)
}

type BaseTask struct {
	SubTask AbsTask

	timeout time.Duration

	readyCMDsLocker sync.Mutex
	readyCMDs       []*Command

	runningCMDsLocker sync.Mutex
	runningCMDs       []*Command

	once         sync.Once
	completeChan chan struct{}
}

func NewBastTask(
	subTask AbsTask,
	timeout time.Duration,
) *BaseTask {
	bt := &BaseTask{
		SubTask: subTask,
		timeout: timeout,
	}
	return bt
}

func (b *BaseTask) GetTaskName() string {
	return b.SubTask.GetTaskName()
}

func (b *BaseTask) GetCommands() []*Command {
	b.readyCMDsLocker.Lock()
	cmds := append([]*Command{}, b.readyCMDs...)
	b.readyCMDs = b.readyCMDs[0:0]
	b.readyCMDsLocker.Unlock()

	b.runningCMDsLocker.Lock()
	b.runningCMDs = append(b.runningCMDs, cmds...)
	b.runningCMDsLocker.Unlock()

	return cmds
}

func (b *BaseTask) getCompleteChan() chan struct{} {
	b.once.Do(func() {
		b.completeChan = make(chan struct{})
	})
	return b.completeChan
}

func (b *BaseTask) GetCompleteChan() <-chan struct{} {
	return b.getCompleteChan()
}

func (b *BaseTask) AppendCommand(targetURL string, callback OnResponseCallback) {
	cmd := b.NewGetCommand(targetURL, callback)
	if cmd == nil {
		return
	}

	b.readyCMDsLocker.Lock()
	defer b.readyCMDsLocker.Unlock()
	b.readyCMDs = append(b.readyCMDs, cmd)
}

func (b *BaseTask) NewCommand(
	method string,
	uri string,
	callback OnResponseCallback,
) *Command {
	req := fasthttp.AcquireRequest()
	req.Header.SetMethod(strings.ToUpper(method))
	req.SetRequestURI(uri)

	ctx := &Context{
		Task:       b.SubTask,
		Request:    req,
		Response:   nil,
		ReqTimeout: b.timeout,
		RespErr:    nil,
	}

	cmd := &Command{
		ctx:      ctx,
		callback: callback,
	}

	return cmd
}

func (b *BaseTask) NewGetCommand(uri string, callback OnResponseCallback) *Command {
	return b.NewCommand("get", uri, callback)
}

func (b *BaseTask) CommandComplete(command *Command) {
	b.runningCMDsLocker.Lock()
	defer b.runningCMDsLocker.Unlock()

	for i := range b.runningCMDs {
		if b.runningCMDs[i] == command {
			b.runningCMDs = append(b.runningCMDs[:i], b.runningCMDs[i+1:]...)
			break
		}
	}

	b.readyCMDsLocker.Lock()
	defer b.readyCMDsLocker.Unlock()
	if len(b.runningCMDs)+len(b.readyCMDs) == 0 {
		close(b.getCompleteChan())
		logrus.WithFields(logrus.Fields{
			"Task": b.SubTask.GetTaskName(),
		}).Info("Task 完成")
	}
}
