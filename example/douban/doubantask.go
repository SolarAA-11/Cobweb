package douban

import (
	"fmt"
	"github.com/SolarDomo/Cobweb/internal/executor"
	"strings"
)

/*
使用 Cobweb 爬取豆瓣电影 Top250 榜单
在 instance 文件夹下建立文件夹 douban
为每部电影 建立格式为
	[排名].[电影名]
文件夹
文件中保存电影封面 cover.jpg
电影详细信息保存到 info.json
*/

type DoubanRule struct {
}

func (r *DoubanRule) InitLinks() []string {
	links := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		links = append(links, fmt.Sprintf("https://movie.douban.com/top250?start=%d&filter=", i*25))
	}
	return links
}

func (r *DoubanRule) InitScrape(ctx *executor.Context) {
	ctx.HTML("#content > div > div.article > ol > li", func(element *executor.HTMLElement) {
		rank := element.ChildText("div.pic em")
		title := element.ChildText("span.title")
		detailLink := element.ChildAttr("div.pic a", "href")
		if strings.TrimSpace(rank) != "65" {
			ctx.Follow(detailLink, r.scrapeDetailPage, executor.H{
				"Rank":  rank,
				"Title": title,
			})
		}
	})
}

func (r *DoubanRule) scrapeDetailPage(ctx *executor.Context) {
	ctx.HTML("", func(element *executor.HTMLElement) {
		title, _ := ctx.Get("Title")
		rank, _ := ctx.Get("Rank")
		picLink := element.ChildAttr("#mainpic > a > img", "src")
		year := element.ChildText("#content > h1 > span.year")
		ctx.SaveResource(picLink, fmt.Sprintf("douban/%v.%v.%v", rank, year, title))
	})
}
