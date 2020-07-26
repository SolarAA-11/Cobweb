package executor

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"golang.org/x/net/html"

	"bytes"
	"io/ioutil"
	"os"
	"path"
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
	fields := logrus.Fields{
		"TaskName":    c.task.Name(),
		"Request":     c.Request,
		"StatusCode":  c.Response.StatusCode(),
		"RespBodyLen": len(c.Response.Body()),
		"RespErr":     c.RespErr,
		"Keys":        c.keys,
	}

	return fields
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

// check if downloader work expected, for example proxy can fetch certain content
// but host ban this proxy is not concerned as invalid situation, i think proxy work normal
// it may can fetch other host's resource, so if context's RespErr which represent whether proxy work well.
func (c *Context) downloaderValid() bool {
	return c.RespErr == nil
}

func (c *Context) Retry() {
	panic(ERR_PROCESS_RETRY)
}

func (c *Context) Doc() (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(c.Response.Body()))
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (c *Context) html(goquerySelector string, callback func(element *HTMLElement)) int {
	if goquerySelector == "" {
		goquerySelector = "html"
	}

	doc, err := c.Doc()
	if err != nil || doc == nil {
		logrus.WithFields(c.LogrusFields()).WithFields(logrus.Fields{
			"GpqueryParseError":                  err,
			LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY: ERR_PROCESS_PARSE_DOC_FAILURE,
		}).Panic()
	}

	nodeCount := doc.Find(goquerySelector).Each(func(index int, selection *goquery.Selection) {
		e := newHTMLElementFromSelectionNode(selection, selection.Nodes[0], c)
		callback(e)
	}).Length()

	return nodeCount
}

func (c *Context) HTML(goquerySelector string, callback func(element *HTMLElement)) {
	nodeCount := c.html(goquerySelector, callback)
	if nodeCount == 0 {
		logrus.WithFields(c.LogrusFields()).WithFields(logrus.Fields{
			"selector":                           goquerySelector,
			LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY: ERR_PROCESS_PARSE_DOC_FAILURE,
		}).Panic()
	}
}

func (c *Context) MayHTML(goquerySelector string, callback func(element *HTMLElement)) {
	c.html(goquerySelector, callback)
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

type HTMLElement struct {
	// element's tag name
	name string

	// element's content text
	text string

	// element's attributes
	attributes []html.Attribute

	dom *goquery.Selection

	ctx *Context
}

func newHTMLElementFromSelectionNode(s *goquery.Selection, node *html.Node, ctx *Context) *HTMLElement {
	return &HTMLElement{
		name:       node.Data,
		text:       goquery.NewDocumentFromNode(node).Text(),
		attributes: node.Attr,
		dom:        s,
		ctx:        ctx,
	}
}

func (e *HTMLElement) Name() string {
	return e.name
}

func (e *HTMLElement) Text() string {
	return e.text
}

func (e *HTMLElement) Attr(key string) string {
	var (
		attr  = ""
		found = false
	)
	for _, attribute := range e.attributes {
		if attribute.Key == key {
			found = true
			attr = attribute.Val
			break
		}
	}
	if !found {
		logrus.WithFields(e.ctx.LogrusFields()).WithFields(logrus.Fields{
			"AttributeKey":                       key,
			LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY: ERR_PROCESS_PARSE_DOC_FAILURE,
		}).Panic()
	}

	return attr
}

func (e *HTMLElement) MayAttr(key string) string {
	var attr = ""
	for _, attribute := range e.attributes {
		if attribute.Key == key {
			attr = attribute.Val
			break
		}
	}
	return attr
}

// return the content string of first child which satisfied selector.
// if no element found, method panic
func (e *HTMLElement) ChildText(selector string) string {
	childNode := e.dom.Find(selector)
	if childNode.Length() == 0 {
		logrus.WithFields(e.ctx.LogrusFields()).WithFields(logrus.Fields{
			"Selector":                           selector,
			LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY: ERR_PROCESS_PARSE_DOC_FAILURE,
		}).Panic()
	}
	return childNode.First().Text()
}

// return the content string of first child which satisfied selector.
// if no element found, method returns empty string
func (e *HTMLElement) MayChildText(selector string) string {
	return e.dom.Find(selector).First().Text()
}

// return array of content string of every child element which satisfied selector.
// method assert at least one element satisfy selector
// if not child element found, method panics
func (e *HTMLElement) ChildTexts(selector string) []string {
	texts := e.MayChildTexts(selector)
	if len(texts) == 0 {
		logrus.WithFields(e.ctx.LogrusFields()).WithFields(logrus.Fields{
			"Selector":                           selector,
			LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY: ERR_PROCESS_PARSE_DOC_FAILURE,
		}).Panic()
	}
	return texts
}

// return array of content string of every child element which satisfied selector.
func (e *HTMLElement) MayChildTexts(selector string) []string {
	return e.dom.Find(selector).Map(func(_ int, selection *goquery.Selection) string {
		return selection.Text()
	})
}

// return attribute value of child element which satisfied selector.
// if acquired attribute is't exist, method panic.
func (e *HTMLElement) ChildAttr(selector, key string) string {
	attr, ok := e.dom.Find(selector).First().Attr(key)
	if !ok {
		logrus.WithFields(e.ctx.LogrusFields()).WithFields(logrus.Fields{
			"Selector":                           selector,
			"AttributeKey":                       key,
			LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY: ERR_PROCESS_PARSE_DOC_FAILURE,
		}).Panic()
	}
	return attr
}

// return attribute value of child element
// return empty string if child does't exist or attribute does't too.
func (e *HTMLElement) MayChildAttr(selector, key string) string {
	attr, ok := e.dom.Find(selector).First().Attr(key)
	if !ok {
		return ""
	}
	return attr
}

// return attributes of children which satisfies selector
// if no child found or some child does't have required attribute, method panic
func (e *HTMLElement) ChildAttrs(selector, key string) []string {
	attrs := e.dom.Find(selector).Map(func(_ int, selection *goquery.Selection) string {
		attr, ok := selection.Attr(key)
		if !ok {
			logrus.WithFields(e.ctx.LogrusFields()).WithFields(logrus.Fields{
				"Selector":                           selector,
				"AttributeKey":                       key,
				LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY: ERR_PROCESS_PARSE_DOC_FAILURE,
			}).Panic()
		}
		return attr
	})
	if len(attrs) == 0 {
		logrus.WithFields(e.ctx.LogrusFields()).WithFields(logrus.Fields{
			"Selector":                           selector,
			"AttributeKey":                       key,
			LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY: ERR_PROCESS_PARSE_DOC_FAILURE,
		}).Panic()
	}
	return attrs
}

// return attributes of children which satisfies selector
// if some child does't have required attribute, the empty string represents its attribute
// May empty array or empty attribute
func (e *HTMLElement) MayChildAttrs(selector, key string) []string {
	attrs := e.dom.Find(selector).Map(func(_ int, selection *goquery.Selection) string {
		attr, ok := selection.Attr(key)
		if !ok {
			return ""
		}
		return attr
	})
	return attrs
}

func (e *HTMLElement) forEach(selector string, callback func(element *HTMLElement)) int {
	return e.dom.Find(selector).Each(func(_ int, selection *goquery.Selection) {
		element := newHTMLElementFromSelectionNode(selection, selection.Nodes[0], e.ctx)
		callback(element)
	}).Length()
}

func (e *HTMLElement) ForEach(selector string, callback func(element *HTMLElement)) {
	nodeLen := e.forEach(selector, callback)
	if nodeLen == 0 {
		logrus.WithFields(e.ctx.LogrusFields()).WithFields(logrus.Fields{
			"Selector":                           selector,
			LOGRUS_PROCESS_ERROR_PANIC_FIELD_KEY: ERR_PROCESS_PARSE_DOC_FAILURE,
		}).Panic()
	}
}

func (e *HTMLElement) MayForEach(selector string, callback func(element *HTMLElement)) {
	e.forEach(selector, callback)
}
