package executor

import (
	"sync"
	"time"
)

type OnResponseCallback func(ctx *Context)

type AbsTask interface {
	GetTaskName() string
	GetCommands() []*Command
	GetCompleteChan() <-chan struct{}
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
	cmd := b.NewCommand(targetURL, callback)
	if cmd == nil {
		return
	}

	b.readyCMDsLocker.Lock()
	defer b.readyCMDsLocker.Unlock()
	b.readyCMDs = append(b.readyCMDs, cmd)
}

func (b *BaseTask) NewCommand(targetURL string, callback OnResponseCallback) *Command {
	cmd := &Command{
		ctx: &Context{
			Request:  NewRequest(targetURL, b.timeout),
			Response: nil,
		},
		callback: callback,
	}
	return cmd
}
