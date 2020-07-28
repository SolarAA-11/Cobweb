package cobweb

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set"

	"github.com/rs/xid"

	"github.com/sirupsen/logrus"
)

const (
	DefaultDownloadTimeout = time.Second * 20
)

type OnParseCallback func(ctx *Context)
type OnDownloadFinishCallback func(ctx *Context)
type OnParseErrorCallback func(info *ParseErrorInfo)
type OnPipeErrorCallback func(info *PipeErrorInfo)

type BaseRule interface {
	InitLinks() []string
	InitParse(ctx *Context)
}

type PipelineRule interface {
	Pipelines() []Pipeline
}

type TaskNameRule interface {
	TaskName() string
}

type DownloadTimeoutRule interface {
	DownloadTimeout() time.Duration
}

type OnDownloadFinishRule interface {
	OnDownloadFinish(ctx *Context)
}

type OnParseErrorRule interface {
	OnParseError(info *ParseErrorInfo)
}

type OnPipeErrorRule interface {
	OnPipeError(info *PipeErrorInfo)
}

type Task struct {
	name string
	id   xid.ID
	rule BaseRule

	downloadTimeout time.Duration
	itemPipelines   []Pipeline

	onParseErrorCallback     OnParseErrorCallback
	onPipeErrorCallback      OnPipeErrorCallback
	onDownloadFinishCallback OnDownloadFinishCallback

	cmdCountLocker    sync.Mutex
	runningCMDCount   int
	completedCMDCount int
	failedCMDCount    int

	itemCountLocker    sync.Mutex
	pipingItemCount    int
	completedItemCount int
	failedItemCount    int

	itemTypeSet mapset.Set

	finishOnce    sync.Once
	finishChannel chan struct{}
}

func newTaskFromRule(rule BaseRule) *Task {
	t := &Task{
		id:            xid.New(),
		rule:          rule,
		itemTypeSet:   mapset.NewSet(),
		finishChannel: make(chan struct{}),
	}
	t.setName(rule)
	t.setDownloadTimeout(rule)
	t.setPipelines(rule)
	t.setParseErrorCallback(rule)
	t.setPipeErrorCallback(rule)
	t.setDownloadFinishCallback(rule)
	return t
}

func (t *Task) setPipelines(rule BaseRule) {
	pipeRule, ok := rule.(PipelineRule)
	if ok {
		t.itemPipelines = pipeRule.Pipelines()
	} else {
		t.itemPipelines = []Pipeline{
			&JsonStdoutPipeline{},
		}
	}
}

func (t *Task) setName(rule BaseRule) {
	nameRule, ok := rule.(TaskNameRule)
	if ok {
		t.name = nameRule.TaskName()
	} else {
		ruleVal := reflect.ValueOf(rule)
		if ruleVal.Kind() == reflect.Ptr {
			ruleVal = ruleVal.Elem()
		}
		if ruleVal.Kind() != reflect.Struct {
			panic("rule has to be a struct")
		}
		t.name = ruleVal.Type().Name()
	}
}

func (t *Task) setDownloadTimeout(rule BaseRule) {
	timeoutRule, ok := rule.(DownloadTimeoutRule)
	if ok {
		t.downloadTimeout = timeoutRule.DownloadTimeout()
	} else {
		t.downloadTimeout = DefaultDownloadTimeout
	}
}

// onParseErrorCallback
func (t *Task) setParseErrorCallback(rule BaseRule) {
	callbackRule, ok := rule.(OnParseErrorRule)
	if ok {
		t.onParseErrorCallback = callbackRule.OnParseError
	} else {
		t.onParseErrorCallback = t.defaultParseErrorCallback
	}
}

// onPipeErrorCallback
func (t *Task) setPipeErrorCallback(rule BaseRule) {
	callbackRule, ok := rule.(OnPipeErrorRule)
	if ok {
		t.onPipeErrorCallback = callbackRule.OnPipeError
	} else {
		t.onPipeErrorCallback = t.defaultPipeErrorCallback
	}
}

// onDownloadFinishCallback
func (t *Task) setDownloadFinishCallback(rule BaseRule) {
	drule, ok := rule.(OnDownloadFinishRule)
	if ok {
		t.onDownloadFinishCallback = drule.OnDownloadFinish
	} else {
		t.onDownloadFinishCallback = t.defaultDownloadFinishCallback
	}
}

func (t *Task) initCommands() []*command {
	links := t.rule.InitLinks()
	cmds := make([]*command, 0, len(links))
	for _, link := range links {
		builder := newCommandBuilder(t)
		builder.Link(link)
		builder.Callback(t.rule.InitParse)
		builder.DownloadTimeout(t.downloadTimeout)
		cmd := builder.build()
		cmds = append(cmds, cmd)
	}
	t.recordNewCommands(cmds)
	return cmds
}

func (t *Task) Wait() {
	<-t.finishChannel
}

func (t *Task) finish() {
	t.finishOnce.Do(func() {
		for _, pipeline := range t.itemPipelines {
			pipeline.Close()
		}
		close(t.finishChannel)
	})
}

func (t *Task) Name() string {
	return t.name
}

func (t *Task) ID() string {
	return t.id.String()
}

func (t *Task) logrusFields() logrus.Fields {
	return logrus.Fields{
		"TaskName":          t.Name(),
		"TaskID":            t.ID(),
		"RunningCMDCnt":     t.runningCMDCount,
		"CompletedCMDCnt":   t.completedCMDCount,
		"FailedCMDCnt":      t.failedCMDCount,
		"PipeliningItemCnt": t.pipingItemCount,
		"CompletedItemCnt":  t.completedItemCount,
		"FailedItemCnt":     t.failedItemCount,
	}
}

func (t *Task) recordNewCommands(cmds []*command) {
	t.cmdCountLocker.Lock()
	defer t.cmdCountLocker.Unlock()
	t.runningCMDCount += len(cmds)
}

func (t *Task) recordCompletedCommand(cmd *command) {
	t.cmdCountLocker.Lock()
	defer t.cmdCountLocker.Unlock()
	t.runningCMDCount--
	t.completedCMDCount++

	t.itemCountLocker.Lock()
	defer t.itemCountLocker.Unlock()
	if t.runningCMDCount == 0 && t.pipingItemCount == 0 {
		t.finish()
	}
}

func (t *Task) recordFailedCommand(cmd *command) {
	t.cmdCountLocker.Lock()
	defer t.cmdCountLocker.Unlock()
	t.runningCMDCount--
	t.failedCMDCount++

	t.itemCountLocker.Lock()
	defer t.itemCountLocker.Unlock()
	if t.runningCMDCount == 0 && t.pipingItemCount == 0 {
		t.finish()
	}
}

func (t *Task) recordNewItemInfos(infos []*itemInfo) {
	t.itemCountLocker.Lock()
	defer t.itemCountLocker.Unlock()
	t.pipingItemCount += len(infos)
}

func (t *Task) recordCompletedItemInfo(info *itemInfo) {
	t.itemCountLocker.Lock()
	defer t.itemCountLocker.Unlock()
	t.pipingItemCount--
	t.completedItemCount++

	t.cmdCountLocker.Lock()
	defer t.cmdCountLocker.Unlock()
	if t.runningCMDCount == 0 && t.pipingItemCount == 0 {
		t.finish()
	}
}

func (t *Task) recordFailedItemInfo(info *itemInfo) {
	t.itemCountLocker.Lock()
	defer t.itemCountLocker.Unlock()
	t.pipingItemCount--
	t.failedItemCount++

	t.cmdCountLocker.Lock()
	defer t.cmdCountLocker.Unlock()
	if t.runningCMDCount == 0 && t.pipingItemCount == 0 {
		t.finish()
	}
}

func (t *Task) recordItemType(itemType reflect.Type) {
	t.itemTypeSet.Add(itemType)
}

func (t *Task) existItemType(itemType reflect.Type) bool {
	return t.itemTypeSet.Contains(itemType)
}

func (t *Task) folderPath() string {
	return path.Join(
		"instance",
		fmt.Sprintf("%v - %v", t.Name(), t.ID()),
	)
}

func (t *Task) pipelines() []Pipeline {
	return t.itemPipelines
}

//  UnknownParseError ParseErrorKind = iota
//	ParseHTMLError
//	HTMLNodeNotFoundError
func (t *Task) defaultParseErrorCallback(info *ParseErrorInfo) {
	switch info.ErrKind {
	case ParseHTMLError:
		info.Ctx.Retry()

	case HTMLNodeNotFoundError:
		info.Ctx.Retry()

	case UnknownParseError:
		t.recordFailedCommand(info.Ctx.cmd)

	default:
		t.recordFailedCommand(info.Ctx.cmd)

	}
}

func (t *Task) debugParseErrorCallback(info *ParseErrorInfo) {
	t.saveParseErrorInfoToInstance(info)
	switch info.ErrKind {
	case ParseHTMLError:
		info.Ctx.Retry()

	case HTMLNodeNotFoundError:
		info.Ctx.Retry()

	case UnknownParseError:
		t.recordFailedCommand(info.Ctx.cmd)

	default:
		t.recordFailedCommand(info.Ctx.cmd)

	}
}

func (t *Task) saveParseErrorInfoToInstance(info *ParseErrorInfo) {
	dirPath := path.Join(
		t.folderPath(),
		"debug",
		"parse",
		fmt.Sprintf("command-%v", info.Ctx.cmd.id),
	)
	os.MkdirAll(dirPath, os.ModeDir)
	ioutil.WriteFile(path.Join(dirPath, "info.json"), info.jsonRep(), os.ModePerm)
	ioutil.WriteFile(path.Join(dirPath, "resource"), info.Ctx.cmd.response().Body(), os.ModePerm)
}

func (t *Task) defaultPipeErrorCallback(info *PipeErrorInfo) {

}

func (t *Task) defaultDownloadFinishCallback(ctx *Context) {

}
