package executor

import (
	"github.com/emirpasic/gods/sets"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"reflect"
	"sync"
	"time"
)

var (
	DEFAULT_REQUEST_TIMEOUT = time.Second * 25
)

type OnResponseCallback func(ctx *Context)
type OnProcessErrorCallback func(cmd *Command, err error)

type BaseRule interface {
	InitLinks() []string
	InitScrape(ctx *Context)
}

// when command finishes download stage, it go into process stage,
// processor invokes command.process in order to parse(scrape) structual data or next scrape link.
// whenever some unexpected situation happened, command.process will panic
// it mean error happened, i use defer and recover to grab this error and handle it
// if you want to custom error handling, let struct while implement BaseRule implement ProcessErrorRule
// otherwise task will use default handler.
type ProcessErrorRule interface {
	OnProcessError(ctx *Command, err error)
}

type TimeoutRule interface {
	RequestTimeout() time.Duration
}

type Task struct {
	rule           BaseRule
	name           string
	requestTimeout time.Duration

	onProcessError OnProcessErrorCallback

	readyCMDSetLocker sync.Mutex
	readyCMDSet       sets.Set

	runningCMDSetLocker sync.Mutex
	runningCMDSet       sets.Set

	completedCMDSetLocker sync.Mutex
	completedCMDSet       sets.Set

	failureCMDsLocker sync.Mutex
	failureCMDSet     sets.Set

	fChanOnce sync.Once
	fChan     chan struct{}

	errCntLocker sync.Mutex
	errCnt       int
}

func newTask(rule BaseRule, name ...string) *Task {
	t := &Task{
		rule:            rule,
		readyCMDSet:     hashset.New(),
		runningCMDSet:   hashset.New(),
		completedCMDSet: hashset.New(),
		failureCMDSet:   hashset.New(),
	}

	// set other option
	t.setTaskName(rule, name...)
	t.setTimeout(rule)
	t.setOnProcessErrorCallback(rule)

	// add init commands
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
		t.readyCMDSet.Clear()
		t.runningCMDSet.Clear()
		t.completedCMDSet.Clear()
		t.failureCMDSet.Clear()
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
	t.readyCMDSetLocker.Lock()
	t.runningCMDSetLocker.Lock()
	defer func() {
		t.readyCMDSetLocker.Unlock()
		t.runningCMDSetLocker.Unlock()
	}()
	vals := t.readyCMDSet.Values()
	cmds := make([]*Command, 0, len(vals))
	for _, val := range vals {
		cmds = append(cmds, val.(*Command))
	}
	t.runningCMDSet.Add(vals...)
	t.readyCMDSet.Clear()
	return cmds
}

func (t *Task) complete(cmd *Command) {
	t.runningCMDSetLocker.Lock()
	defer t.runningCMDSetLocker.Unlock()
	t.runningCMDSet.Remove(cmd)

	//t.completedCMDsLocker.Lock()
	//t.completedCMDs = append(t.completedCMDs, cmd)
	//t.completedCMDsLocker.Unlock()

	t.readyCMDSetLocker.Lock()
	defer t.readyCMDSetLocker.Unlock()
	if t.readyCMDSet.Empty() && t.runningCMDSet.Empty() {
		t.finish()
	}
}

func (t *Task) addCommands(cmds ...*Command) {
	t.readyCMDSetLocker.Lock()
	defer t.readyCMDSetLocker.Unlock()
	vals := make([]interface{}, 0, len(cmds))
	for _, cmd := range cmds {
		vals = append(vals, cmd)
	}
	t.readyCMDSet.Add(vals...)
}

func (t *Task) failure(cmd *Command) {
	// todo
	t.complete(cmd)
}

// runningCMDBackToReady
func (t *Task) runningCMDBackToReady(cmd *Command) {
	t.runningCMDSetLocker.Lock()
	defer t.runningCMDSetLocker.Unlock()

	if t.runningCMDSet.Contains(cmd) {
		t.runningCMDSet.Remove(cmd)
	} else {
		return
	}

	t.readyCMDSetLocker.Lock()
	defer t.readyCMDSetLocker.Unlock()

	t.readyCMDSet.Add(cmd)
}

func (t *Task) addCommandByUri(uri *fasthttp.URI, callback OnResponseCallback, contextInfo ...H) {
	t.addCommands(newCommandByURI(uri, callback, t, contextInfo...))
}

func (t *Task) addCommandByUriString(uri string, callback OnResponseCallback, contextInfo ...H) {
	t.addCommands(newCommandByURIString(uri, callback, t, contextInfo...))
}

func (t *Task) setTimeout(rule interface{}) {
	timeoutRule, ok := rule.(TimeoutRule)
	if ok {
		t.requestTimeout = timeoutRule.RequestTimeout()
	} else {
		t.requestTimeout = DEFAULT_REQUEST_TIMEOUT
	}
}

func (t *Task) setTaskName(rule interface{}, name ...string) {
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
	t.name = taskName
}

func (t *Task) setOnProcessErrorCallback(rule interface{}) {
	processErrRule, ok := rule.(ProcessErrorRule)
	if ok {
		t.onProcessError = processErrRule.OnProcessError
	} else {
		t.onProcessError = defaultOnProcessErrorCallback
	}
}

func defaultOnProcessErrorCallback(cmd *Command, err error) {
	logEntry := logrus.WithFields(cmd.ctx.LogrusFields()).WithField("Error", err)
	switch err {
	case ERR_PROCESS_RETRY:
		logrus.WithFields(logrus.Fields{
			"Proxy": cmd.ctx.downloader.proxy(),
		}).WithFields(cmd.ctx.LogrusFields()).Errorf("command require retry.")
		cmd.ctx.downloader.banned(cmd.ctx)
		cmd.ctx.task.runningCMDBackToReady(cmd)
	case ERR_PROCESS_PARSE_DOC_FAILURE:
		logEntry.Error("doc parse fail")
		cmd.ctx.downloader.banned(cmd.ctx)
		cmd.ctx.task.runningCMDBackToReady(cmd)
	default:
		logEntry.Error("other failure")
		cmd.ctx.task.failure(cmd)
	}
}
