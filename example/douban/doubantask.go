package douban

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
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
*/

type DoubanRule struct {
}

func (r *DoubanRule) InitLinks() []string {
	links := make([]string, 0, 10)
	for i := 0; i < 1; i++ {
		links = append(links, fmt.Sprintf("https://movie.douban.com/top250?start=%d&filter=", i*25))
	}
	return links
}

func (r *DoubanRule) InitScrape(ctx *executor.Context) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(ctx.Response.Body()))
	if err != nil {
		fmt.Println("BadRequest")
		ctx.Retry()
	}
	l := doc.Find("#content > div > div.article > ol > li > div").Each(func(i int, s *goquery.Selection) {
		link, ok := s.Find("div.pic > a").Attr("href")
		if !ok {
			fmt.Println("BadRequest")
			ctx.Retry()
		}

		rank := s.Find("div.pic > em").Text()
		ctx.Follow(link, r.scrapeDetailPage, executor.H{
			"rank": rank,
		})
	}).Length()
	if l == 0 {
		fmt.Println("BadRequest")
		ctx.Retry()
	}
}

func (r *DoubanRule) scrapeDetailPage(ctx *executor.Context) {
	doc := ctx.Doc()
	if doc == nil {
		ctx.Retry()
	}

	title := doc.Find("#content > h1 > span:first-child").Text()
	if len(strings.TrimSpace(title)) == 0 {
		ctx.Retry()
	}
	rank, _ := ctx.Get("rank")
	coverLink, ok := doc.Find("#mainpic > a > img").Attr("src")
	if !ok {
		ctx.Retry()
	}
	ctx.SaveResource(coverLink, fmt.Sprintf("douban/%v.%v/cover", rank, title))
}
