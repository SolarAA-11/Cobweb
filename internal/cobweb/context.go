package cobweb

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"reflect"

	"github.com/PuerkitoBio/goquery"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"golang.org/x/net/html"
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
	c.panicByNeedRetry(logrus.Fields{})
}

// send item to be handled by pipelines
// invalid item type will trigger process error panic
func (c *Context) Item(item interface{}) {
	// check item type valid
	// process error item type invalid error panic if invalid
	itemType := reflect.TypeOf(item)
	valid, err := c.task.itemTypeCheckByPrev(itemType)
	if err != nil {
		valid = c.checkItemTypeValid(itemType)
		if valid {
			c.task.itemValidTypeRegister(itemType)
		} else {
			c.task.itemInvalidTypeRegister(itemType)
		}
	}

	if !valid {
		c.panicByPipeItemTypeInvalid(logrus.Fields{
			"Item": item,
		})
	}

	// create itemInfo and add to task
	itemInfo := &ItemInfo{
		task: c.task,
		item: item,
	}
	c.task.addItemInfo(itemInfo)
}

// check whether item's type is valid. Type is valid only if
// 1. item's Type is Struct or Pointer of Struct.
// 2. Struct's Open Field' Type is string or []string.
// 3. if Struct's Open Field's Type is not string or []string but Struct or Pointer of Struct, check it recursively
//
// if item's Type is valid return true, otherwise returning false
func (c *Context) checkItemTypeValid(itemType reflect.Type) bool {
	if itemType.Kind() == reflect.Ptr {
		itemType = itemType.Elem()
	}

	if itemType.Kind() != reflect.Struct {
		return false
	}

	itemFieldCnt := itemType.NumField()
	for i := 0; i < itemFieldCnt; i++ {
		field := itemType.Field(i)
		if field.Name[0] >= 'A' && field.Name[0] <= 'Z' {
			switch field.Type.Kind() {
			case reflect.Ptr:
				if !c.checkItemTypeValid(field.Type) {
					return false
				}
			case reflect.Struct:
				if !c.checkItemTypeValid(field.Type) {
					return false
				}
			case reflect.String:
				continue
			case reflect.Slice:
				if field.Type.Elem().Kind() == reflect.String {
					continue
				} else {
					return false
				}
			default:
				return false
			}
		}
	}
	return true
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
		c.panicByDocParseError(logrus.Fields{
			"GpqueryParseError": err,
		})
		return -1
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
		c.panicByDocParseError(logrus.Fields{
			"selector": goquerySelector,
		})
	}
}

func (c *Context) MayHTML(goquerySelector string, callback func(element *HTMLElement)) {
	c.html(goquerySelector, callback)
}

func (c *Context) panicByNeedRetry(logFields logrus.Fields) {
	panic(newProcessPanicInfo(logrus.WithFields(logFields), PROCESS_ERR_NEED_RETRY))
}

func (c *Context) panicByDocParseError(logFields logrus.Fields) {
	panic(newProcessPanicInfo(logrus.WithFields(logFields), PROCESS_ERR_PARSE_DOC_FAILURE))
}

func (c *Context) panicByPipeItemTypeInvalid(logFields logrus.Fields) {
	panic(newProcessPanicInfo(logrus.WithFields(logFields), PROCESS_ERR_ITEM_TYPE_INVALID))
}

func (c *Context) saveResponseBodyForDebug() {
	if len(c.Response.Body()) == 0 {
		// todo

	}

	//fileName := fmt.Sprintf("debug-respBodySaved-%v-")
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
		e.ctx.panicByDocParseError(logrus.Fields{
			"AttributeKey": key,
		})
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

// return the content string of child which satisfied selector.
// if no element found, method panic
func (e *HTMLElement) ChildText(selector string) string {
	childNode := e.dom.Find(selector)
	if childNode.Length() == 0 {
		e.ctx.panicByDocParseError(logrus.Fields{
			"Selector": selector,
		})
	}
	return childNode.Text()
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
		e.ctx.panicByDocParseError(logrus.Fields{
			"Selector": selector,
		})
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
		e.ctx.panicByDocParseError(logrus.Fields{
			"Selector":     selector,
			"AttributeKey": key,
		})
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
			e.ctx.panicByDocParseError(logrus.Fields{
				"Selector":     selector,
				"AttributeKey": key,
			})
		}
		return attr
	})
	if len(attrs) == 0 {
		e.ctx.panicByDocParseError(logrus.Fields{
			"Selector":     selector,
			"AttributeKey": key,
		})
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
		e.ctx.panicByDocParseError(logrus.Fields{
			"Selector": selector,
		})
	}
}

func (e *HTMLElement) MayForEach(selector string, callback func(element *HTMLElement)) {
	e.forEach(selector, callback)
}