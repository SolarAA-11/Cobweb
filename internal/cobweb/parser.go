package cobweb

import (
	"runtime"
	"sync"

	"github.com/sirupsen/logrus"
)

//
type parser struct {
	inCMDChannel       <-chan *command
	outCMDChannel      chan<- *command
	outItemInfoChannel chan<- *itemInfo

	stopOnce    sync.Once
	stopWg      sync.WaitGroup
	stopChannel chan struct{}
}

func newParser(
	inCMDChannel <-chan *command,
	outCMDChannel chan<- *command,
	outItemInfoChannel chan<- *itemInfo,
) *parser {
	p := &parser{
		inCMDChannel:       inCMDChannel,
		outCMDChannel:      outCMDChannel,
		outItemInfoChannel: outItemInfoChannel,
		stopOnce:           sync.Once{},
		stopWg:             sync.WaitGroup{},
		stopChannel:        make(chan struct{}),
	}
	for i := 0; i < runtime.NumCPU(); i++ {
		go p.parseRoutine(i)
	}
	return p
}

func (p *parser) stop() {
	p.stopOnce.Do(func() {
		logrus.Info("parser is stopping...")
		close(p.stopChannel)
	})
	p.stopWg.Wait()
	logrus.Info("parser has stopped.")
}

// get parse command from inCMDChannel
// run command's parse method
// get new command and item info send to outCMDChannel and outItemInfoChannel
func (p *parser) parseRoutine(routineID int) {
	p.stopWg.Add(1)
	defer func() {
		p.stopWg.Done()
	}()
	logEntry := logrus.WithField("RoutineID", routineID)

	var loop = true
	for loop {
		select {
		case cmd, ok := <-p.inCMDChannel:
			if !ok {
				logEntry.Fatal("inCMDChannel has been closed. Bad usage.")
			}

			newCMDs, newItemInfos := cmd.parse()
			if !cmd.isUnderFailCntLimit() {
				cmd.task.recordFailedCommand(cmd)
				continue
			} else if cmd.needRetry {
				newCMDs = append(newCMDs, cmd)
				cmd.needRetry = false
			}

			go p.sendNewCommands(newCMDs)
			go p.sendNewItemInfo(newItemInfos)
		case <-p.stopChannel:
			loop = false
		}
	}

}

// send new command to outCMDChannel
func (p *parser) sendNewCommands(cmds []*command) {
	p.stopWg.Add(1)
	defer p.stopWg.Done()

	stopped := false
	for _, cmd := range cmds {
		select {
		case <-p.stopChannel:
			stopped = true
		default:
			p.outCMDChannel <- cmd
		}
		if stopped {
			break
		}
	}
}

// send new item info to outItemInfoChannel
func (p *parser) sendNewItemInfo(itemInfos []*itemInfo) {
	stopped := false
	for _, item := range itemInfos {
		select {
		case <-p.stopChannel:
			stopped = true
		default:
			p.outItemInfoChannel <- item
		}
		if stopped {
			break
		}
	}
}
