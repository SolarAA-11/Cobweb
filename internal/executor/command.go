package executor

import (
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
	logrus.WithFields(c.ctx.LogrusFields()).Debug("Command object released.")
}

func (c *Command) process() {
	defer func() {
		if err := recover(); err != nil {
			if err == ERR_RETRY {
				c.ctx.task.retryCommand(c)
			}
		} else {
			c.ctx.task.complete(c)
		}
	}()

	c.callback(c.ctx)
}
