package executor

import (
	"bytes"
	"errors"
	"github.com/PuerkitoBio/goquery"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"math"
	"os"
	"path"
)

var (
	ERR_RETRY = errors.New("command need retry")
)

type H map[string]interface{}

func (h *H) clone() H {
	newH := make(H)
	for key, val := range *h {
		newH[key] = val
	}
	return newH
}

type Context struct {
	task       *Task
	Request    *fasthttp.Request
	Response   *fasthttp.Response
	RespErr    error
	downloader Downloader
	retry      bool
	keys       H
}

func (c *Context) Get(key string) (interface{}, bool) {
	v, ok := c.keys[key]
	return v, ok
}

func (c *Context) Set(key string, v interface{}) {
	c.keys[key] = v
}

func (c *Context) LogrusFields() logrus.Fields {
	return logrus.Fields{
		"TaskName":    c.task.Name(),
		"Request":     c.Request,
		"StatusCode":  c.Response.StatusCode(),
		"RespBodyLen": len(c.Response.Body()),
		"RespErr":     c.RespErr,
		"Keys":        c.keys,
	}
}

func (c *Context) Follow(uri string, callback OnResponseCallback, info ...H) {
	switch len(info) {
	case 0:
		break
	case 1:
		for key, val := range info[0] {
			c.Set(key, val)
		}
	default:
		panic("info's len large than 1")
	}

	c.task.addCommandByUriString(uri, callback, c.keys)
}

// check if command's download response data is valid
func (c *Context) responseValid() bool {
	if c.RespErr != nil {
		return false
	}
	return c.Response != nil && c.Response.StatusCode() == 200
}

func (c *Context) Retry() {
	logrus.WithFields(logrus.Fields{
		"Proxy": c.downloader.proxy(),
	}).WithFields(c.LogrusFields()).Debug("Context require retry.")
	c.downloader.increaseErrCnt(math.MaxInt32)
	panic(ERR_RETRY)
}

func (c *Context) Doc() *goquery.Document {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(c.Response.Body()))
	if err != nil {
		return nil
	}
	return doc
}

func (c *Context) SaveResource(link, fileName string) {
	c.Follow(link, saveResourceCallback, H{
		"FileName": fileName,
	})
}

func saveResourceCallback(ctx *Context) {
	val, _ := ctx.Get("FileName")
	fileName := val.(string)
	fileName = path.Join("instance", fileName)
	err := os.MkdirAll(path.Dir(fileName), os.ModeDir)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"FileName": fileName,
			"Error":    err,
		}).WithFields(ctx.LogrusFields()).Error("Save Resource Error")
		return
	}
	err = ioutil.WriteFile(fileName, ctx.Response.Body(), os.ModePerm)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"FileName": fileName,
			"Error":    err,
		}).WithFields(ctx.LogrusFields()).Error("Save Resource Error")
		return
	}
	logrus.WithFields(logrus.Fields{
		"FileName": fileName,
	}).WithFields(ctx.LogrusFields()).Info("Resource saved.")
}
