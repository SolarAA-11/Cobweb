package executor

import (
	"errors"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"runtime"
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
}

func (c *Command) processDeferFunc() {
	panicVal := recover()
	if panicVal != nil {
		// recover from panic
		switch panicVal.(type) {
		case error:
			c.ctx.task.onProcessError(c, panicVal.(error))
		case *logrus.Entry:
			// panic from logrus.Panic
			entry := panicVal.(*logrus.Entry)
			panicErrVal, ok := entry.Data[LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY]
			if !ok {
				// not panic from process error defined
				c.ctx.task.onProcessError(c, errors.New(entry.Message))
			} else {
				panicErr, ok := panicErrVal.(error)
				if ok {
					c.ctx.task.onProcessError(c, panicErr)
				} else {
					// logrus fields key LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY
					// its value is not error type
					// fatal situation
					logrus.WithFields(c.ctx.LogrusFields()).WithFields(logrus.Fields{
						"PanicVal":                             panicVal,
						"LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY": panicErr,
					}).Fatal("LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY Value is not error type")
				}
			}
		default:
			logrus.WithFields(c.ctx.LogrusFields()).WithFields(logrus.Fields{
				"PanicVal": panicVal,
			}).Error("Process Panic with unexpect situation, we recover routine and think this command is failed.")
			c.ctx.task.failure(c)
		}
	} else {
		// process normal return
		c.ctx.task.complete(c)
	}

}
