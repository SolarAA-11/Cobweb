package cobweb

import (
	"runtime"

	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
)

type Command struct {
	ctx      *Context
	callback OnResponseCallback
}

func newCommandByURI(uri *fasthttp.URI, callback OnResponseCallback, t *Task, contextInfo ...H) *Command {

	req := fasthttp.AcquireRequest()
	req.SetRequestURI(uri.String())
	resp := fasthttp.AcquireResponse()
	ctx := &Context{
		task:     t,
		Request:  req,
		Response: resp,
		RespErr:  nil,
	}
	switch len(contextInfo) {
	case 0:
		ctx.keys = make(H)
	case 1:
		ctx.keys = contextInfo[0].clone()
	default:
		panic("contextInfo large than 1")
	}

	cmd := &Command{ctx: ctx, callback: callback}
	runtime.SetFinalizer(cmd, (*Command).releaseResource)
	return cmd
}

func newCommandByURIString(uri string, callback OnResponseCallback, t *Task, contextInfo ...H) *Command {
	u := fasthttp.AcquireURI()
	defer fasthttp.ReleaseURI(u)

	err := u.Parse(nil, []byte(uri))
	if err != nil {
		return nil
	}
	return newCommandByURI(u, callback, t, contextInfo...)
}

func (c *Command) releaseResource() {
	fasthttp.ReleaseRequest(c.ctx.Request)
	fasthttp.ReleaseResponse(c.ctx.Response)
}

func (c *Command) Retry() {
	c.ctx.Retry()
}

// after fini
func (c *Command) process() {
	defer c.processDeferFunc()
	c.callback(c.ctx)

	// pipe ItemInfos to pipelines in order to handling item.
	items := c.ctx.task.extractItemInfos()
	pipes := c.ctx.task.pipelines()
	for _, item := range items {
		for _, pipe := range pipes {
			pipe.Pipe(item)
		}
	}
}

// if process panics by cobweb inner method
// recover will return processPanicInfo's pointer
type processPanicInfo struct {
	// logrusInfo save some information about panic situation
	// like, CSS selector string for ERR_PARSE_DOC_FAILURE
	// and Context Info
	logrusInfo *logrus.Entry
	pErr       ProcessError
}

func newProcessPanicInfo(logrusInfo *logrus.Entry, err ProcessError) *processPanicInfo {
	pInfo := &processPanicInfo{
		logrusInfo: logrusInfo,
		pErr:       err,
	}
	return pInfo
}

func (c *Command) processDeferFunc() {
	panicVal := recover()
	if panicVal != nil {
		// process panics!
		switch panicVal.(type) {
		case *processPanicInfo:
			pInfo, _ := panicVal.(*processPanicInfo)
			c.ctx.task.onProcessError(c, pInfo.pErr, pInfo.logrusInfo)
		default:
			c.ctx.task.onProcessError(c, PROCESS_ERR_UNKNOWN, logrus.WithField("OriginalError", panicVal))
		}
	} else {
		// process finished
		c.ctx.task.complete(c)
	}
}
