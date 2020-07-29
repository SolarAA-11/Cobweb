package zhihu

import (
	"fmt"

	"github.com/SolarDomo/Cobweb/internal/cobweb"
)

type zhihuRule struct {
}

func (r *zhihuRule) InitLinks() []string {
	return []string{
		"https://www.zhihu.com/hot",
	}
}

func (r *zhihuRule) InitParse(ctx *cobweb.Context) {
	ctx.HTML("", func(element *cobweb.HTMLElement) {
		titles := element.ChildrenTexts("#TopstoryContent > div > div > div.HotList-list > section > div.HotItem-content > a > h2")
		fmt.Println(titles)
	})
}
