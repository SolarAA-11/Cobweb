package cobweb

import (
	"encoding/json"
	"runtime"
	"time"

	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
)

type ParseErrorKind int

func (kind ParseErrorKind) String() string {
	switch kind {
	case ParseHTMLError:
		return "ParseHTMLError"
	case HTMLNodeNotFoundError:
		return "HTMLNodeNotFoundError"
	case UnknownParseError:
		return "UnknownParseError"
	default:
		return "Unknown"
	}
}

const (
	UnknownParseError ParseErrorKind = iota
	ParseHTMLError
	HTMLNodeNotFoundError
)

type ParseErrorInfo struct {
	Ctx        *Context
	ErrKind    ParseErrorKind
	PanicValue interface{}
}

func (i *ParseErrorInfo) jsonRep() []byte {
	d := i.Ctx.logrusFields()
	d["PanicValue"] = i.PanicValue
	d["ParseErrorKind"] = i.ErrKind
	j, err := json.MarshalIndent(d, "", "\t")
	if err != nil {
		// todo
	}
	return j
}

type PipeErrorKind int

const (
	UnknownPipeError PipeErrorKind = iota
)

type PipeErrorInfo struct {
	Ctx       *Context
	ErrKind   PipeErrorKind
	PanicInfo interface{}
}

type commandBuilder struct {
	task *Task

	// for build request
	link      string
	userAgent string
	cookies   map[string]string

	parseCallback   OnParseCallback
	downloadTimeout time.Duration

	// for context data
	contextData H
}

func newCommandBuilder(task *Task) *commandBuilder {
	return &commandBuilder{
		task:            task,
		userAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.97 Safari/537.36",
		downloadTimeout: DefaultDownloadTimeout,
		cookies:         make(map[string]string),
		contextData:     make(H),
	}
}

func (b *commandBuilder) Link(link string) *commandBuilder {
	b.link = link
	return b
}

func (b *commandBuilder) UserAgent(agent string) *commandBuilder {
	b.userAgent = agent
	return b
}

func (b *commandBuilder) Cookie(key, val string) *commandBuilder {
	b.cookies[key] = val
	return b
}

func (b *commandBuilder) Callback(callback OnParseCallback) *commandBuilder {
	b.parseCallback = callback
	return b
}

func (b *commandBuilder) DownloadTimeout(timeout time.Duration) *commandBuilder {
	b.downloadTimeout = timeout
	return b
}

func (b *commandBuilder) ContextData(data H) *commandBuilder {
	for key, val := range data {
		b.contextData[key] = val
	}
	return b
}

func (b *commandBuilder) build() *command {
	cmd := &command{
		id:              xid.New(),
		task:            b.task,
		onParseCallback: b.parseCallback,
		downloadTimeout: b.downloadTimeout,
		contextData:     b.contextData.clone(),
	}

	// build request
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(b.link)
	req.Header.SetUserAgent(b.userAgent)
	for k, v := range b.cookies {
		req.Header.SetCookie(k, v)
	}
	cmd.downloadRequest = req

	// response
	resp := fasthttp.AcquireResponse()
	cmd.downloadResponse = resp

	// SetFinalizer to release request and response
	runtime.SetFinalizer(cmd, (*command).finalizer)

	return cmd
}

type command struct {
	id xid.ID

	task            *Task
	onParseCallback OnParseCallback

	downloaderUsed *downloader

	downloadRequest  *fasthttp.Request
	downloadResponse *fasthttp.Response
	downloadError    error
	downloadTimeout  time.Duration

	// context extra info data
	contextData H

	//
	needRetry bool
}

func (c *command) finalizer() {
	fasthttp.ReleaseRequest(c.downloadRequest)
	fasthttp.ReleaseResponse(c.downloadResponse)
}

func (c *command) logrusFields() logrus.Fields {
	return logrus.Fields{
		"CommandID":   c.id,
		"Request":     c.downloadRequest.String(),
		"RespBodyLen": len(c.downloadResponse.Body()),
		"StatusCode":  c.downloadResponse.StatusCode(),
		"DownloadErr": c.downloadError,
		"Task":        c.task.logrusFields(),
	}
}

func (c *command) retry() {
	c.needRetry = true
}

func (c *command) request() *fasthttp.Request {
	return c.downloadRequest
}

func (c *command) beBanned() {
	if c.downloaderUsed != nil {
		c.downloaderUsed.beBaned(c)
	}
}

func (c *command) response() *fasthttp.Response {
	return c.downloadResponse
}

func (c *command) timeout() time.Duration {
	return c.downloadTimeout
}

func (c *command) downloadFinished(downloadError error) bool {
	c.task.onDownloadFinishCallback(c.createContext())
	c.downloadError = downloadError
	//return c.downloadError == nil
	return c.downloadError == nil && c.downloadResponse.StatusCode() == 200
}

// use coreCommand to handle context, parse required resource to generate new command and itemInfo
// new command and itemInfo are saved in context, task will record these information
func (c *command) parse() ([]*command, []*itemInfo) {
	defer c.parseDeferFunc()

	ctx := c.createContext()
	c.onParseCallback(ctx)

	itemInfos := ctx.itemInfos()
	cmds := ctx.commands()

	// task record these information
	c.task.recordNewCommands(cmds)
	c.task.recordNewItemInfos(itemInfos)

	// if function run here it's obvious that the command is completed
	c.task.recordCompletedCommand(c)

	return cmds, itemInfos
}

func (c *command) parseDeferFunc() {
	panicVal := recover()
	if panicVal == nil {
		// parse method return normal
		return
	}

	// parse method panics
	switch panicVal.(type) {
	case *ParseErrorInfo:
		c.task.onParseErrorCallback(panicVal.(*ParseErrorInfo))

	default:
		// panic by unexpected situation
		c.task.onParseErrorCallback(&ParseErrorInfo{
			Ctx:        c.createContext(),
			ErrKind:    UnknownParseError,
			PanicValue: panicVal,
		})
	}
}

func (c *command) pipe(info *itemInfo) {
	pipelines := c.task.pipelines()
	for _, pipeline := range pipelines {
		pipeline.Pipe(info)
	}
	c.task.recordCompletedItemInfo(info)
}

func (c *command) pipeDeferFunc() {
	panicVal := recover()
	if panicVal == nil {
		return
	}

	// pipe method panic
	switch panicVal.(type) {
	case *PipeErrorInfo:
		c.task.onPipeErrorCallback(panicVal.(*PipeErrorInfo))
	default:
		// panic by unexpected situation
		c.task.onPipeErrorCallback(&PipeErrorInfo{
			Ctx:       c.createContext(),
			ErrKind:   UnknownPipeError,
			PanicInfo: panicVal,
		})
	}
}

// create new context with command=
func (c *command) createContext() *Context {
	return newContext(c)
}
