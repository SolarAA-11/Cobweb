package cobweb

import (
	"io/ioutil"
	"testing"

	"github.com/rs/xid"
	"github.com/valyala/fasthttp"
)

type testSuits struct {
	t *testing.T
}

func NewTestSuits(t *testing.T) *testSuits {
	suit := &testSuits{t: t}
	return suit
}

func (suit *testSuits) withContext(callback OnParseCallback, ctx *Context) ([]string, []interface{}) {
	defer suit.parseDeferFunc()
	callback(ctx)

	items := make([]interface{}, 0, len(ctx.iInfos))
	for _, info := range ctx.iInfos {
		items = append(items, info.item)
	}
	links := make([]string, 0, len(ctx.cmds))
	for _, cmd := range ctx.cmds {
		links = append(links, cmd.request().URI().String())
	}
	return links, items
}

func (suit *testSuits) WithString(callback OnParseCallback, context string, contextData H) ([]string, []interface{}) {
	return suit.WithBytes(callback, []byte(context), contextData)
}

func (suit *testSuits) WithFile(callback OnParseCallback, filename string, contextData H) ([]string, []interface{}) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		suit.t.Error(err)
	}
	return suit.WithBytes(callback, b, contextData)
}

func (suit *testSuits) WithBytes(callback OnParseCallback, data []byte, contextData H) ([]string, []interface{}) {
	cmd := &command{
		id: xid.New(),
		task: &Task{
			name: "TestSuit",
			id:   xid.New(),
		},
		contextData: contextData,
	}
	// build request
	req := fasthttp.AcquireRequest()
	cmd.downloadRequest = req

	// build response
	resp := fasthttp.AcquireResponse()
	resp.SetBody(data)
	cmd.downloadResponse = resp

	ctx := newContext(cmd)
	return suit.withContext(callback, ctx)
}

func (suit *testSuits) parseDeferFunc() {
	panicVal := recover()
	if panicVal == nil {
		// test success.
		return
	}

	switch panicVal.(type) {
	case (*ParseErrorInfo):
		info := panicVal.(*ParseErrorInfo)
		suit.handlePanicInfo(info)
	default:
		suit.t.Errorf("ParseCallback Panic With Cobweb Unexpecet Error, panic value: %v", panicVal)
	}
}

func (suit *testSuits) handlePanicInfo(info *ParseErrorInfo) {
	switch info.ErrKind {
	case ParseHTMLError:
		suit.t.Errorf(string(info.jsonRep()))
	case HTMLNodeNotFoundError:
		suit.t.Errorf(string(info.jsonRep()))
	case UnknownParseError:
		suit.t.Errorf(string(info.jsonRep()))
	}
}
