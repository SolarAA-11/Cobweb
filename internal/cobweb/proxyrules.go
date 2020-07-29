package cobweb

import (
	"fmt"
	"strings"
)

var proxyFetchRuleCreators = []func() BaseRule{
	//newKuidailiRule,
	//newYundailiRule,
	//newMianfeidaili89Rule,
	//newKaixindailiRule,
	//newXiaohuandailiRule,
}

// https://www.kuaidaili.com/free/inha/1/
type kuaidailiRule struct {
}

func newKuidailiRule() BaseRule {
	return &kuaidailiRule{}
}

func (r *kuaidailiRule) InitLinks() []string {
	links := make([]string, 0)
	for i := 0; i < 10; i++ {
		links = append(links, fmt.Sprintf("https://www.kuaidaili.com/free/inha/%v/", i))
	}
	return links
}

func (r *kuaidailiRule) InitParse(ctx *Context) {
	ctx.HTML("#list > table > tbody > tr", func(element *HTMLElement) {
		tdTexts := element.ChildrenTexts("td")
		proxy := &Proxy{
			Host:  tdTexts[0],
			Port:  tdTexts[1],
			HTTPS: tdTexts[3] == "HTTP",
		}
		if tdTexts[2] == "高匿名" {
			proxy.Anonymity = Anonymous
		} else {
			proxy.Anonymity = Transparent
		}

		err := ProxyStorageSingleton().CreateProxy(proxy)
		if err != nil {
			// todo
		}
	})
}

// http://www.ip3366.net/?stype=1&page=1
type yundailiRule struct {
}

func newYundailiRule() BaseRule {
	return &yundailiRule{}
}

func (y *yundailiRule) InitLinks() []string {
	links := make([]string, 0)
	for i := 0; i < 10; i++ {
		links = append(links, fmt.Sprintf("http://www.ip3366.net/?stype=1&page=%v", i))
	}
	return links
}

func (y *yundailiRule) InitParse(ctx *Context) {
	ctx.HTML("#list > table > tbody > tr", func(element *HTMLElement) {
		tds := element.ChildrenTexts("td")
		proxy := &Proxy{
			ID:    0,
			Host:  tds[0],
			Port:  tds[1],
			HTTPS: strings.ToLower(tds[3]) == "https",
		}
		if tds[2] == "高匿代理IP" {
			proxy.Anonymity = Anonymous
		} else {
			proxy.Anonymity = Transparent
		}
		err := ProxyStorageSingleton().CreateProxy(proxy)
		if err != nil {
			// todo
		}
	})
}

// http://www.89ip.cn/index_1.html
type mianfeidaili89Rule struct {
}

func newMianfeidaili89Rule() BaseRule {
	return &mianfeidaili89Rule{}
}

func (m *mianfeidaili89Rule) InitLinks() []string {
	links := make([]string, 0)
	for i := 0; i < 20; i++ {
		links = append(links, fmt.Sprintf("http://www.89ip.cn/index_%v.html", i))
	}
	return links
}

func (m *mianfeidaili89Rule) InitParse(ctx *Context) {
	ctx.HTML("div.layui-row.layui-col-space15 > div.layui-col-md8 > div > div.layui-form > table > tbody > tr", func(element *HTMLElement) {
		tds := element.ChildrenTexts("td")
		for i := range tds {
			tds[i] = strings.TrimSpace(tds[i])
		}

		proxy := &Proxy{
			Host:      tds[0],
			Port:      tds[1],
			HTTPS:     false,
			Anonymity: Transparent,
		}
		err := ProxyStorageSingleton().CreateProxy(proxy)
		if err != nil {
			// todo
		}
	})
}

// http://www.goubanjia.com/
// todo 端口号是假的 需要查看 js 部分
type quanwangdailiRule struct {
}

func (r *quanwangdailiRule) InitLinks() []string {
	return []string{"http://www.goubanjia.com/"}
}

func (r *quanwangdailiRule) InitParse(ctx *Context) {
	ctx.HTML("#services > div > div.row > div > div > div > table > tbody > tr", func(element *HTMLElement) {

	})
}

// http://www.kxdaili.com/dailiip/1/1.html
type kaixindailiRule struct {
}

func newKaixindailiRule() BaseRule {
	return &kaixindailiRule{}
}

func (r *kaixindailiRule) InitLinks() []string {
	links := make([]string, 0)
	for i := 0; i < 5; i++ {
		links = append(links, fmt.Sprintf("http://www.kxdaili.com/dailiip/1/%v.html", i+1))
	}
	return links
}

func (r *kaixindailiRule) InitParse(ctx *Context) {
	ctx.HTML("body > div.banner-box > div.header-container > div.domain-block.price-block > div.auto > div.hot-product > div.hot-product-content > table > tbody > tr", func(element *HTMLElement) {
		tds := element.ChildrenTexts("td")
		proxy := &Proxy{
			Host:  tds[0],
			Port:  tds[1],
			HTTPS: strings.Contains(strings.ToLower(tds[3]), "https"),
		}
		if tds[2] == "高匿" {
			proxy.Anonymity = Anonymous
		} else {
			proxy.Anonymity = Transparent
		}
		err := ProxyStorageSingleton().CreateProxy(proxy)
		if err != nil {
			// todo
		}
	})
}

// https://ip.ihuan.me/
// todo 抓的太慢放弃
type xiaohuandailiRule struct {
	cnt int
}

func newXiaohuandailiRule() BaseRule {
	return &xiaohuandailiRule{}
}

func (r *xiaohuandailiRule) InitLinks() []string {
	return []string{"https://ip.ihuan.me/"}
}

func (r *xiaohuandailiRule) InitParse(ctx *Context) {
	ctx.HTML("div.col-md-10 > div.table-responsive > table > tbody > tr", func(element *HTMLElement) {
		tds := element.ChildrenTexts("td")
		tds = trimStrings(tds)
		proxy := &Proxy{
			ID:    0,
			Host:  tds[0],
			Port:  tds[1],
			HTTPS: tds[4] == "支持",
		}
		if tds[6] == "高匿" {
			proxy.Anonymity = Elite
		} else if tds[6] == "普匿" {
			proxy.Anonymity = Anonymous
		} else {
			proxy.Anonymity = Transparent
		}
		ProxyStorageSingleton().CreateProxy(proxy)
	})
	ctx.HTML("div.col-md-10 > nav > ul", func(element *HTMLElement) {
		link := element.MayChildAttr("a[aria-label=Next]", "href")
		if link != "" {
			val, ok := ctx.Get("CNT")
			var cnt int
			if !ok {
				cnt = 1
			} else {
				cnt = val.(int)
			}
			if cnt < 5 {
				ctx.Follow(link, r.InitParse, H{"CNT": cnt + 1})
			}
		}
	})
}

func trimStrings(strs []string) []string {
	rets := make([]string, 0, len(strs))
	for i := range strs {
		rets = append(rets, strings.TrimSpace(strs[i]))
	}
	return rets
}
