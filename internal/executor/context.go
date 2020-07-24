package executor

import (
	"github.com/valyala/fasthttp"
	"time"
)

type Context struct {
	Task       AbsTask
	Request    *fasthttp.Request
	Response   *fasthttp.Response
	ReqTimeout time.Duration
	RespErr    error
}

func (c *Context) RequestURI() string {
	if c.Request == nil {
		return ""
	}
	return string(c.Request.RequestURI())
}
