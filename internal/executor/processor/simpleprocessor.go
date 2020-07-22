package processor

import (
	"github.com/SolarDomo/Cobweb/internal/executor/command"
	"runtime"
	"sync"
)

type SimpleProcessorFactory struct {
}

func (this *SimpleProcessorFactory) NewProcessor(
	commandChannel <-chan command.AbsCommand,
) (AbsProcessor, <-chan command.AbsCommand) {
	processor := &simpleProcessor{
		fanOutCmdChannel: commandChannel,
		fanInCmdChannel:  make(chan command.AbsCommand),
		wg:               sync.WaitGroup{},
		stopSyncOnce:     sync.Once{},
	}
	for i := 0; i < runtime.NumCPU(); i++ {
		go processor.workerRoutine(i)
	}
	return processor, processor.fanInCmdChannel
}

// 简单的处理器
//
type simpleProcessor struct {
	fanOutCmdChannel <-chan command.AbsCommand
	fanInCmdChannel  chan command.AbsCommand

	wg           sync.WaitGroup
	stopSyncOnce sync.Once
}

func (this *simpleProcessor) workerRoutine(routineID int) {
	this.wg.Add(1)
	defer this.wg.Done()

	for cmd := range this.fanOutCmdChannel {
		resultCmdList := cmd.Process()
		for _, resultCmd := range resultCmdList {
			this.fanInCmdChannel <- resultCmd
		}
	}
}

func (this *simpleProcessor) GetProcessorName() string {
	return "SimpleProcessor"
}

func (this *simpleProcessor) WaitAndStop() {
	this.stopSyncOnce.Do(func() {
		this.wg.Wait()
		close(this.fanInCmdChannel)
	})
}
