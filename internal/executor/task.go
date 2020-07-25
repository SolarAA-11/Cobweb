package executor

import (
	mapset "github.com/deckarep/golang-set"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"reflect"
	"sync"
	"time"
)

type OnResponseCallback func(ctx *Context)

const (
	DEFAULT_REQUEST_TIMEOUT = time.Second * 25
)

type BaseRule interface {
	InitLinks() []string
	InitScrape(ctx *Context)
}

type TimeoutRule interface {
	RequestTimeout() time.Duration
}

type Task struct {
	rule BaseRule
	name string

	requestTimeout time.Duration

	set mapset.Set

	readyCMDsLocker sync.Mutex
	readyCMDs       []*Command

	runningCMDsLocker sync.Mutex
	runningCMDs       []*Command

	completedCMDsLocker sync.Mutex
	completedCMDs       []*Command

	fChanOnce sync.Once
	fChan     chan struct{}

	errCntLocker sync.Mutex
	errCnt       int
}

func newTask(rule BaseRule, name ...string) *Task {
	taskName := ""
	switch len(name) {
	case 0:
		ruleVal := reflect.ValueOf(rule)
		if ruleVal.Kind() == reflect.Ptr {
			ruleVal = ruleVal.Elem()
		}
		if ruleVal.Kind() != reflect.Struct {
			panic("rule has to be a struct")
		}
		taskName = ruleVal.Type().Name()
	case 1:
		taskName = name[0]
	default:
		panic("age name's len can not large than 1.")
	}

	t := &Task{
		rule: rule,
		name: taskName,
	}

	// set other option
	t.setTimeout(rule)

	links := t.rule.InitLinks()
	for _, link := range links {
		t.addCommandByUriString(link, t.rule.InitScrape)
	}

	return t
}

func (t *Task) Name() string {
	return t.name
}

func (t *Task) Wait() {
	<-t.finishedChan()
}

func (t *Task) finishedChan() chan struct{} {
	t.fChanOnce.Do(func() {
		t.fChan = make(chan struct{})
	})
	return t.fChan
}

func (t *Task) finish() {
	select {
	case _, ok := <-t.finishedChan():
		if ok {
			panic("bad usage of finished channel")
		} else {
			panic("double close finished channel")
		}
	default:
		t.readyCMDs = nil
		t.runningCMDs = nil
		close(t.finishedChan())
	}
}

func (t *Task) increaseErrCnt() {
	t.errCntLocker.Lock()
	defer t.errCntLocker.Unlock()
	t.errCnt++
}

func (t *Task) ErrCnt() int {
	t.errCntLocker.Lock()
	defer t.errCntLocker.Unlock()
	return t.errCnt
}

func (t *Task) extractCommands() []*Command {
	t.readyCMDsLocker.Lock()
	t.runningCMDsLocker.Lock()
	defer func() {
		t.runningCMDs = append(t.runningCMDs, t.readyCMDs...)
		t.readyCMDs = t.readyCMDs[0:0]
		t.readyCMDsLocker.Unlock()
		t.runningCMDsLocker.Unlock()
	}()
	return t.readyCMDs
}

func (t *Task) complete(cmd *Command) {
	t.runningCMDsLocker.Lock()
	defer t.runningCMDsLocker.Unlock()

	cmdIndex := 0
	for ; cmdIndex < len(t.runningCMDs); cmdIndex++ {
		if t.runningCMDs[cmdIndex] == cmd {
			break
		}
	}
	if cmdIndex == len(t.runningCMDs) {
		logrus.WithFields(cmd.ctx.LogrusFields()).Fatal("cmd is not in runningCMDs.")
		return
	}

	t.runningCMDs = append(t.runningCMDs[:cmdIndex], t.runningCMDs[cmdIndex+1:]...)

	//t.completedCMDsLocker.Lock()
	//t.completedCMDs = append(t.completedCMDs, cmd)
	//t.completedCMDsLocker.Unlock()

	t.readyCMDsLocker.Lock()
	defer t.readyCMDsLocker.Unlock()
	if len(t.readyCMDs)+len(t.runningCMDs) == 0 {
		t.finish()
	}
}

func (t *Task) addCommands(cmds ...*Command) {
	t.readyCMDsLocker.Lock()
	defer t.readyCMDsLocker.Unlock()
	t.readyCMDs = append(t.readyCMDs, cmds...)
}

func (t *Task) retryCommand(cmd *Command) {
	t.runningCMDsLocker.Lock()
	defer t.runningCMDsLocker.Unlock()

	cmdIndex := 0
	for ; cmdIndex < len(t.runningCMDs); cmdIndex++ {
		if t.runningCMDs[cmdIndex] == cmd {
			break
		}
	}
	if cmdIndex == len(t.runningCMDs) {
		logrus.WithFields(cmd.ctx.LogrusFields()).Fatal("cmd is not in runningCMDs.")
		return
	}

	t.runningCMDs = append(t.runningCMDs[:cmdIndex], t.runningCMDs[cmdIndex+1:]...)

	t.readyCMDsLocker.Lock()
	defer t.readyCMDsLocker.Unlock()
	t.readyCMDs = append(t.readyCMDs, cmd)
}

func (t *Task) addCommandByUri(uri *fasthttp.URI, callback OnResponseCallback, contextInfo ...H) {
	t.addCommands(newCommandByURI(uri, callback, t, contextInfo...))
}

func (t *Task) addCommandByUriString(uri string, callback OnResponseCallback, contextInfo ...H) {
	t.addCommands(newCommandByURIString(uri, callback, t, contextInfo...))
}

func (t *Task) setTimeout(ruleInterface interface{}) {
	timeoutRule, ok := ruleInterface.(TimeoutRule)
	if ok {
		t.requestTimeout = timeoutRule.RequestTimeout()
	} else {
		t.requestTimeout = DEFAULT_REQUEST_TIMEOUT
	}
}
