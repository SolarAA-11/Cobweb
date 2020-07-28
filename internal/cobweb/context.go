package cobweb

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"reflect"

	"github.com/SolarDomo/Cobweb/pkg/utils"

	"github.com/PuerkitoBio/goquery"

	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
)

type H map[string]interface{}

func (h H) clone() H {
	newH := make(H)
	for k, v := range h {
		newH[k] = v
	}
	return newH
}

func (h H) union(o H) H {
	newh := h.clone()
	for k, v := range o {
		newh[k] = v
	}
	return newh
}

type Context struct {
	cmd    *command
	doc    *goquery.Document
	cmds   []*command
	iInfos []*itemInfo
	data   H
}

func newContext(
	cmd *command,
) *Context {
	return &Context{
		cmd:  cmd,
		data: cmd.contextData.clone(),
	}
}

func (c *Context) Set(key string, value interface{}) {
	c.data[key] = value
}

func (c *Context) Get(key string) (interface{}, bool) {
	val, ok := c.data[key]
	return val, ok
}

func (c *Context) logrusFields() logrus.Fields {
	return utils.LogrusFiledsUnion(
		c.cmd.logrusFields(),
		logrus.Fields{
			"ContextData": c.data,
		},
	)
}

func (c *Context) NewFollowBuilder() *FollowBuilder {
	b := &FollowBuilder{
		newCommandBuilder(c.cmd.task),
	}
	b.ContextData(c.data)
	return b
}

func (c *Context) Follow(link string, callback OnParseCallback, data ...H) {
	fBuilder := c.NewFollowBuilder()
	switch len(data) {
	case 1:
		fBuilder.ContextData(data[0])
	case 0:
	default:
		logrus.WithFields(logrus.Fields{
			"ExtraData": data,
		}).WithFields(c.logrusFields()).Fatal("argument extraData len large than 1")
	}
	c.FollowWithBuilder(link, callback, fBuilder)
}

func (c *Context) FollowWithBuilder(link string, callback OnParseCallback, fBuilder *FollowBuilder) {
	uri := fasthttp.AcquireURI()
	defer fasthttp.ReleaseURI(uri)
	c.cmd.downloadRequest.URI().CopyTo(uri)
	uri.Update(link)
	fBuilder.link = uri.String()

	fBuilder.Callback(callback)

	cmd := fBuilder.build()
	c.cmds = append(c.cmds, cmd)
}

func (c *Context) Retry() {
	c.cmd.beBanned()
	c.cmd.retry()
}

// add new item in order to be pipelined
func (c *Context) Item(item interface{}) {
	itemType := reflect.TypeOf(item)
	if !c.checkItemType(itemType) {
		logrus.WithFields(c.logrusFields()).WithFields(logrus.Fields{
			"Item": item,
		}).Fatal("invalid item type")
	} else {
		c.cmd.task.recordItemType(itemType)
	}

	c.iInfos = append(c.iInfos, &itemInfo{
		ctx:  c,
		item: item,
	})
}

// check if item type is valid
// 1. itemType is a struct or a pointer of struct
// 2. every Open Field is string, []string, valid struct or pointer of valid struct
func (c *Context) checkItemType(itemType reflect.Type) bool {
	if itemType.Kind() == reflect.Ptr {
		itemType = itemType.Elem()
	}

	if itemType.Kind() != reflect.Struct {
		return false
	}

	numFields := itemType.NumField()
	for i := 0; i < numFields; i++ {
		structField := itemType.Field(i)
		if !(structField.Name[0] >= 'A' && structField.Name[0] <= 'Z') {
			// not open field
			continue
		}

		switch structField.Type.Kind() {
		case reflect.String:
			continue
		case reflect.Slice:
			if structField.Type.Elem().Kind() != reflect.String {
				// field is a slice but not slice of string
				// invalid
				return false
			}
		case reflect.Ptr:
			c.checkItemType(structField.Type)
		case reflect.Struct:
			c.checkItemType(structField.Type)
		default:
			return false
		}
	}

	return true
}

func (c *Context) Doc() *goquery.Document {
	if c.doc != nil {
		return c.doc
	}

	doc, err := c.MayDoc()
	if err != nil {
		c.panicByHTMLParseError(err)
	}

	return doc
}

func (c *Context) MayDoc() (*goquery.Document, error) {
	if c.doc != nil {
		return c.doc, nil
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(c.cmd.downloadResponse.Body()))
	if err != nil {
		return nil, err
	}

	c.doc = doc
	return doc, nil
}

func (c *Context) HTML(selector string, callback func(element *HTMLElement)) int {
	cnt, err := c.MayHTML(selector, callback)
	if err != nil {
		c.panicByHTMLParseError(err)
	} else if cnt == 0 {
		c.panicByHTMLNotFound(htmlSelectRules{}.append("selector", selector))
	}

	return cnt
}

func (c *Context) MayHTML(selector string, callback func(element *HTMLElement)) (int, error) {
	if selector == "" {
		selector = "html"
	}
	doc, err := c.MayDoc()
	if err != nil {
		return 0, err
	}

	cnt := doc.Find(selector).Each(func(_ int, selection *goquery.Selection) {
		callback(newHTMLElement(c, selection, htmlSelectRules{}.append("selector", selector)))
	}).Length()

	return cnt, nil
}

// save link's resource to instance/[taskName].[taskID]/[fileName]
func (c *Context) SaveResource(link string, fileName string) {
	c.Follow(link, c.saveResourceCallback, H{
		"Cobweb-FileName": fileName,
	})
}

func (c *Context) saveResourceCallback(ctx *Context) {
	if ctx.cmd.response().StatusCode() != 200 {
		ctx.Retry()
		return
	}

	val, ok := ctx.Get("Cobweb-FileName")
	if !ok {
		logrus.WithFields(ctx.logrusFields()).Error("save resource failed, Cobweb-FileName does't exist")
		return
	}

	fileName, ok := val.(string)
	if !ok {
		logrus.WithFields(ctx.logrusFields()).WithField("val", fileName).Error("save resource failed, Cobweb-FileName is't string")
		return
	}

	filePath := path.Join(c.cmd.task.folderPath(), fileName)
	if err := os.MkdirAll(path.Dir(filePath), os.ModeDir); err != nil {
		logrus.WithFields(ctx.logrusFields()).WithField("Error", err).Error("save resource failed")
		return
	}

	if err := ioutil.WriteFile(filePath, ctx.cmd.downloadResponse.Body(), os.ModePerm); err != nil {
		logrus.WithFields(ctx.logrusFields()).WithField("Error", err).Error("save resource failed")
		return
	}

	logrus.WithField("FilePath", filePath).Info("resource saved")
}

func (c *Context) commands() []*command {
	return c.cmds
}

func (c *Context) itemInfos() []*itemInfo {
	return c.iInfos
}

func (c *Context) panicByHTMLParseError(err error) {
	panic(&ParseErrorInfo{
		Ctx:        c,
		ErrKind:    ParseHTMLError,
		PanicValue: err,
	})
}

// want to find HTML Element or Element's attribute, but can not
// selectRule is the rule to find it
func (c *Context) panicByHTMLNotFound(selectRule htmlSelectRules) {
	panic(&ParseErrorInfo{
		Ctx:        c,
		ErrKind:    HTMLNodeNotFoundError,
		PanicValue: selectRule,
	})
}

// Builder pattern for Context.FollowWithBuilder
type FollowBuilder struct {
	*commandBuilder
}

type htmlSelectRules map[string][]string

func (r htmlSelectRules) append(key, rule string) htmlSelectRules {
	r[key] = append(r[key], rule)
	return r
}

func (r htmlSelectRules) clone() htmlSelectRules {
	newKeyRules := make(htmlSelectRules)
	for key, rules := range r {
		newKeyRules[key] = make([]string, len(rules))
		for _, rule := range rules {
			newKeyRules[key] = append(newKeyRules[key], rule)
		}
	}
	return newKeyRules
}

// represent for one element in html doc
type HTMLElement struct {
	selection   *goquery.Selection
	ctx         *Context
	selectRules htmlSelectRules
}

func newHTMLElement(ctx *Context, selection *goquery.Selection, selectRule htmlSelectRules) *HTMLElement {
	return &HTMLElement{
		selection:   selection,
		ctx:         ctx,
		selectRules: selectRule.clone(),
	}
}

func (e *HTMLElement) Text() string {
	return e.selection.Text()
}

func (e *HTMLElement) Attr(key string) string {
	attr, exists := e.mayAttr(key)
	if !exists {
		e.ctx.panicByHTMLNotFound(e.selectRules.append("attrname", key))
	}
	return attr
}

func (e *HTMLElement) MayAttr(key string) string {
	attr, ok := e.mayAttr(key)
	if !ok {
		return ""
	}
	return attr
}

func (e *HTMLElement) mayAttr(key string) (string, bool) {
	return e.selection.Attr(key)
}

func (e *HTMLElement) ChildText(selector string) string {
	text, ok := e.mayChildText(selector)
	if !ok {
		e.ctx.panicByHTMLNotFound(e.selectRules.append("selector", selector))
	}
	return text
}

func (e *HTMLElement) MayChildText(selector string) string {
	text, _ := e.mayChildText(selector)
	return text
}

func (e *HTMLElement) mayChildText(selector string) (string, bool) {
	childSelection := e.selection.Find(selector).First()
	if childSelection.Length() == 0 {
		return "", false
	}
	return childSelection.Text(), true
}

func (e *HTMLElement) ChildAttr(selector, key string) string {
	childSelection := e.selection.Find(selector).First()
	if childSelection.Length() == 0 {
		e.ctx.panicByHTMLNotFound(e.selectRules.append("selector", selector))
	}

	attr, ok := childSelection.Attr(key)
	if !ok {
		e.ctx.panicByHTMLNotFound(e.selectRules.append("selector", selector).append("attrname", selector))
	}

	return attr
}

func (e *HTMLElement) MayChildAttr(selector, key string) string {
	childSelection := e.selection.Find(selector).First()
	if childSelection.Length() == 0 {
		return ""
	}

	attr, ok := childSelection.Attr(key)
	if !ok {
		return ""
	}

	return attr
}

func (e *HTMLElement) ChildrenTexts(selector string) []string {
	texts, ok := e.mayChildrenTexts(selector)
	if !ok {
		e.ctx.panicByHTMLNotFound(e.selectRules.append("selector", selector))
	}
	return texts
}

func (e *HTMLElement) MayChildrenTexts(selector string) []string {
	texts, _ := e.mayChildrenTexts(selector)
	return texts
}

func (e *HTMLElement) mayChildrenTexts(selector string) ([]string, bool) {
	childrenSelection := e.selection.Find(selector)
	if childrenSelection.Length() == 0 {
		return nil, false
	}
	texts := childrenSelection.Map(func(_ int, selection *goquery.Selection) string {
		return selection.Text()
	})
	return texts, true
}

func (e *HTMLElement) ChildrenAttrs(selector, attrname string) []string {
	attrs, ok := e.mayChildrenAttrs(selector, attrname)
	if !ok {
		e.ctx.panicByHTMLNotFound(e.selectRules.append("selector", selector).append("attrname", selector))
	}
	return attrs
}

func (e *HTMLElement) MayChildrenAttrs(selector, attrname string) []string {
	attrs, _ := e.mayChildrenAttrs(selector, attrname)
	return attrs
}

func (e *HTMLElement) mayChildrenAttrs(selector, attrname string) ([]string, bool) {
	childrenSelection := e.selection.Find(selector)
	if childrenSelection.Length() == 0 {
		return nil, false
	}

	someNoteDoesNotHaveTargetAttr := false
	attrs := childrenSelection.Map(func(_ int, selection *goquery.Selection) string {
		attr, ok := selection.Attr(attrname)
		if !ok {
			someNoteDoesNotHaveTargetAttr = true
			return ""
		}

		return attr
	})

	if someNoteDoesNotHaveTargetAttr {
		return nil, false
	} else {
		return attrs, true
	}
}

func (e *HTMLElement) ForEach(selector string, callback func(element *HTMLElement)) {
	ok := e.mayForEach(selector, callback)
	if !ok {
		e.ctx.panicByHTMLNotFound(e.selectRules.append("selector", selector))
	}
}

func (e *HTMLElement) MayForEach(selector string, callback func(element *HTMLElement)) {
	e.mayForEach(selector, callback)
}

func (e *HTMLElement) mayForEach(selector string, callback func(element *HTMLElement)) bool {
	return e.selection.Find(selector).Each(func(_ int, selection *goquery.Selection) {
		callback(newHTMLElement(e.ctx, selection, e.selectRules.append("selector", selector)))
	}).Length() > 0
}
