package meizitu

import (
	"fmt"

	"github.com/SolarDomo/Cobweb/internal/cobweb"
	"github.com/valyala/fasthttp"
)

// https://www.lnlnl.cn/meizitu/
type MeizituRule struct {
}

func (r *MeizituRule) InitLinks() []string {
	links := make([]string, 0)
	for i := 0; i < 1; i++ {
		links = append(links, fmt.Sprintf("https://www.lnlnl.cn/meizitu/%v/", i+1))
	}
	return links
}

func (r *MeizituRule) InitParse(ctx *cobweb.Context) {
	ctx.HTML("", func(element *cobweb.HTMLElement) {
		links := element.ChildrenAttrs("#main > div > div > div > ul > li > div > a", "href")
		for _, link := range links {
			ctx.Follow(link, r.parseDetailPage)
		}
	})
}

func (r *MeizituRule) parseDetailPage(ctx *cobweb.Context) {
	ctx.HTML("", func(element *cobweb.HTMLElement) {
		title := element.ChildText("#main > div > div.mainl > div.post > div.title > h1")
		imgLinks := element.ChildrenAttrs("#main > div > div.mainl > div.post > div.article_content.text p img", "src")
		for index, link := range imgLinks {
			ctx.SaveResource(link, fmt.Sprintf("%v/%v.%v", title, index, getExtention(link)))
		}
		nextPage := element.MayChildAttr("#nextpage", "href")
		if nextPage != "" {
			ctx.Follow(nextPage, r.parseDetailPage)
		}
	})
}

func getExtention(urlstring string) string {
	uri := fasthttp.AcquireURI()
	defer fasthttp.ReleaseURI(uri)
	err := uri.Parse(nil, []byte(urlstring))
	if err != nil {
		return ""
	}
	ext := string(uri.LastPathSegment())
	if len(ext) != 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	return ext
}
