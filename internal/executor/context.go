package executor

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"sync"
	"time"
)

type Context struct {
	BaseTask   *BaseTask
	Request    *fasthttp.Request
	Response   *fasthttp.Response
	ReqTimeout time.Duration
	RespErr    error

	retry bool

	keys sync.Map
}

func (c *Context) Set(key string, val interface{}) {
	c.keys.Store(key, val)
}

func (c *Context) Get(key string) (interface{}, bool) {
	return c.keys.Load(key)
}

func (c *Context) String() string {
	return fmt.Sprintf("BaseTask=( %s ), Req=( %s ), Resp=( %s ), RespErr=( %s )",
		c.BaseTask.GetTaskName(),
		c.Request,
		c.Response,
		c.RespErr,
	)
}

func (c *Context) Retry() {
	c.retry = true
}

func (c *Context) LogrusFields() logrus.Fields {
	fs := logrus.Fields{
		"TaskName": c.BaseTask.GetTaskName(),
		"Request":  c.Request,
		"RespErr":  c.RespErr,
	}

	if c.Response != nil {
		fs["StatusCode"] = c.Response.StatusCode()
		fs["BodyLen"] = len(c.Response.Body())
	} else {
		fs["Response"] = nil
	}

	return fs
}

func (c *Context) Follow(url string, callback OnResponseCallback) {
	uri := fasthttp.AcquireURI()
	c.Request.URI().CopyTo(uri)
	uri.Update(url)

	//c.BaseTask.
}
