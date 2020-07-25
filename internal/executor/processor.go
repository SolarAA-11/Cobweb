package executor

import (
	"github.com/sirupsen/logrus"
	"runtime"
	"sync"
)

type processor struct {
	inCMDCh  chan *Command
	outCMDCh chan *Command

	stopCh chan struct{}

	stopOnce sync.Once
	wg       sync.WaitGroup
}

func newProcessor(inCMDChan chan *Command, outCMDChan chan *Command) *processor {
	p := &processor{
		inCMDCh:  inCMDChan,
		outCMDCh: outCMDChan,
		stopCh:   make(chan struct{}),
	}

	for i := 0; i < runtime.NumCPU(); i++ {
		go p.processRoutine(i)
	}

	return p
}

func (p *processor) processRoutine(id int) {
	p.wg.Add(1)
	defer p.wg.Done()

	logEntry := logrus.WithFields(logrus.Fields{"ID": id})
	logEntry.Debug("Start process routine.")

	var loop = true
	for loop {
		select {
		case cmd, ok := <-p.inCMDCh:
			if !ok {
				logEntry.Warn("Process routine is handling closed command channel. This routine will return")
				loop = false
			}
			cmd.process()
			go func() {
				p.wg.Add(1)
				defer p.wg.Done()

				newCMDs := cmd.ctx.task.extractCommands()
				for _, newCMD := range newCMDs {
					p.outCMDCh <- newCMD
				}
			}()
		case <-p.stopCh:
			loop = false
		}
	}

	logEntry.Info("Process routine stopped.")
}

func (p *processor) Stop() {
	p.stopOnce.Do(func() {
		logrus.Info("processor stopping...")
		close(p.stopCh)
	})
	p.wg.Wait()
	logrus.Info("processor stopped.")
}
