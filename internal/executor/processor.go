package executor

import (
	"github.com/sirupsen/logrus"
	"runtime"
	"sync"
)

type Processor struct {
	cmdInChan  <-chan *Command
	cmdOutChan chan<- *Command

	once      sync.Once
	wg        sync.WaitGroup
	closeChan chan struct{}
}

func newProcessor(inChan <-chan *Command, outChan chan<- *Command) *Processor {
	p := &Processor{
		cmdInChan:  inChan,
		cmdOutChan: outChan,
		closeChan:  make(chan struct{}),
	}

	for i := 0; i < runtime.NumCPU(); i++ {
		go p.workRoutine(i)
	}

	return p
}

func (p *Processor) workRoutine(routineID int) {
	logEntry := logrus.WithFields(logrus.Fields{
		"ID": routineID,
	})
	logEntry.Debug("启动 Processor 协程")

	var loop = true
	for loop {
		select {
		case cmd, ok := <-p.cmdInChan:
			if !ok {
				continue
			}

			newCommandSlice := cmd.Process()
			go p.outputCommands(newCommandSlice)

		case <-p.closeChan:
			loop = false
		}
	}

	logEntry.Debug("关闭 Processor 协程")
}

func (p *Processor) outputCommands(commands []*Command) {
	for _, command := range commands {
		p.cmdOutChan <- command
	}
}

func (p *Processor) WaitAndStop() {
	logrus.Debug("开始关闭 Processor")
	p.once.Do(func() {
		close(p.closeChan)
	})
	p.wg.Wait()
	logrus.Info("已经关闭 Processor")
}
