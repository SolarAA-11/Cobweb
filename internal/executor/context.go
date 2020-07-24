package executor

import (
	"fmt"
	"github.com/sirupsen/logrus"
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

func (c *Context) String() string {
	return fmt.Sprintf("Task=( %s ), Req=( %s ), Resp=( %s ), RespErr=( %s )",
		c.Task.GetTaskName(),
		c.Request,
		c.Response,
		c.RespErr,
	)
}

func (c *Context) LogrusFields() logrus.Fields {
	fs := logrus.Fields{
		"Task":    c.Task.GetTaskName(),
		"Request": c.Request,
		"RespErr": c.RespErr,
	}

	if c.Response != nil {
		fs["StatusCode"] = c.Response.StatusCode()
		fs["BodyLen"] = len(c.Response.Body())
	} else {
		fs["Response"] = nil
	}

	return fs
}
