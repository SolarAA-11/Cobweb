package cobweb

import (
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/emirpasic/gods/sets"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
)

var (
	DEFAULT_REQUEST_TIMEOUT = time.Second * 25
)

type OnResponseCallback func(ctx *Context)
type OnProcessErrorCallback func(cmd *Command, err ProcessError, entry *logrus.Entry)

type BaseRule interface {
	InitLinks() []string
	InitScrape(ctx *Context)
}

// when command finishes download stage, it goes into process stage,
// processor invokes command.process in order to parse(scrape) structual data or next scrape link.
// whenever some unexpected situation happened, command.process will panic
// it mean error happened, i use defer and recover to grab this error and handle it
// if you want to custom error handling, let struct while implement BaseRule implement ProcessErrorRule
// otherwise task will use default handler.
type ProcessErrorRule interface {
	OnProcessError(cmd *Command, err ProcessError, entry *logrus.Entry)
}

type TimeoutRule interface {
	RequestTimeout() time.Duration
}

type PipelineRule interface {
	Pipelines() []Pipeline
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

	failureCMDSetLocker sync.Mutex
	failureCMDSet       sets.Set

	fChanOnce sync.Once
	fChan     chan struct{}

	itemInfosLocker sync.Mutex
	itemInfos       []*ItemInfo

	// itemTypesRegister saves checked item type
	// valid and invalid item type
	itemTypesRegister sync.Map

	pipes []Pipeline

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
	t.setPipeline(rule)

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
		// notify task's pipeline task finished, close pipeline.
		for _, pipe := range t.pipes {
			pipe.Close()
		}

		t.readyCMDSet.Clear()
		t.runningCMDSet.Clear()
		t.completedCMDSet.Clear()
		t.failureCMDSet.Clear()
		close(t.finishedChan())
	}
}

// check itemType by previous checked item
func (t *Task) itemTypeCheckByPrev(itemType reflect.Type) (bool, error) {
	valid, ok := t.itemTypesRegister.Load(itemType)
	if !ok {
		return false, errors.New("not exists")
	} else {
		return valid.(bool), nil
	}
}

// register valid item type
func (t *Task) itemValidTypeRegister(itemType reflect.Type) {
	t.itemTypesRegister.Store(itemType, true)
}

// register invalid item type
func (t *Task) itemInvalidTypeRegister(itemType reflect.Type) {
	t.itemTypesRegister.Store(itemType, false)
}

// add new ItemInfo to Task obj
func (t *Task) addItemInfo(info *ItemInfo) {
	t.itemInfosLocker.Lock()
	defer t.itemInfosLocker.Unlock()
	t.itemInfos = append(t.itemInfos, info)
}

// extract ItemInfos in Task obj now.
func (t *Task) extractItemInfos() []*ItemInfo {
	t.itemInfosLocker.Lock()
	defer func() {
		t.itemInfos = t.itemInfos[0:0]
		t.itemInfosLocker.Unlock()
	}()
	extractedInfos := make([]*ItemInfo, len(t.itemInfos))
	copy(extractedInfos, t.itemInfos)
	return extractedInfos
}

func (t *Task) pipelines() []Pipeline {
	return t.pipes
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

// running command result is failure
// move command from runningCMDSet to failureCMDSet
func (t *Task) failure(cmd *Command) {
	t.runningCMDSetLocker.Lock()
	defer t.runningCMDSetLocker.Unlock()

	if t.runningCMDSet.Contains(cmd) {
		t.runningCMDSet.Remove(cmd)
	} else {
		return
	}

	t.failureCMDSetLocker.Lock()
	defer t.failureCMDSetLocker.Unlock()

	t.failureCMDSet.Add(cmd)
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

func (t *Task) setPipeline(rule interface{}) {
	pipeRule, ok := rule.(PipelineRule)
	if ok {
		t.pipes = pipeRule.Pipelines()
	} else {
		t.pipes = []Pipeline{
			&StdoutPipeline{},
		}
	}
}

func (t *Task) setOnProcessErrorCallback(rule interface{}) {
	processErrRule, ok := rule.(ProcessErrorRule)
	if ok {
		t.onProcessError = processErrRule.OnProcessError
	} else {
		t.onProcessError = defaultOnProcessErrorCallback
	}
}

func defaultOnProcessErrorCallback(cmd *Command, err ProcessError, entry *logrus.Entry) {
	entry.WithFields(cmd.ctx.LogrusFields()).WithFields(logrus.Fields{
		"Error": err.Error(),
		"Proxy": cmd.ctx.downloader.proxy(),
	}).Debug("process error")
	switch err {
	case PROCESS_ERR_ITEM_TYPE_INVALID:
		entry.WithFields(cmd.ctx.LogrusFields()).Fatal("BAD USAGE PIPELINE ITEM TYPE INVALID")

	case PROCESS_ERR_PARSE_DOC_FAILURE:
		cmd.ctx.downloader.banned(cmd.ctx)
		cmd.ctx.task.runningCMDBackToReady(cmd)

	case PROCESS_ERR_NEED_RETRY:
		cmd.ctx.downloader.banned(cmd.ctx)
		cmd.ctx.task.runningCMDBackToReady(cmd)

	case PROCESS_ERR_UNKNOWN:
		cmd.ctx.task.failure(cmd)

	default:
		entry.WithFields(cmd.ctx.LogrusFields()).Fatal("BAD USAGE")
	}
}
