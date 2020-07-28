package douban

import (
	"fmt"
	"strings"

	"github.com/SolarDomo/Cobweb/internal/cobweb"
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

type DoubanItem struct {
	Title     string
	Year      string
	PicLink   string
	Rank      string
	Directors []string
	Actors    []string
	Kinds     []string
	Runtime   string
	IMDbLink  string
}

type DoubanRule struct {
}

func (r *DoubanRule) Pipelines() []cobweb.Pipeline {
	return []cobweb.Pipeline{
		&cobweb.JsonFilePipeline{},
	}
}

func (r *DoubanRule) InitLinks() []string {
	links := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		links = append(links, fmt.Sprintf("https://movie.douban.com/top250?start=%d&filter=", i*25))
	}
	return links
}

func (r *DoubanRule) InitParse(ctx *cobweb.Context) {
	ctx.HTML("#content > div > div.article > ol > li", func(element *cobweb.HTMLElement) {
		rank := element.ChildText("div.pic em")
		detailLink := element.ChildAttr("div.pic a", "href")
		if strings.TrimSpace(rank) != "65" {
			ctx.Follow(detailLink, r.scrapeDetailPage, cobweb.H{
				"Rank": rank,
			})
		}
	})
}

func (r *DoubanRule) scrapeDetailPage(ctx *cobweb.Context) {
	ctx.HTML("", func(element *cobweb.HTMLElement) {
		rank, _ := ctx.Get("Rank")
		title := element.ChildText("#content > h1 > span:nth-child(1)")
		actors := element.MayChildrenTexts("#info > span.actor > span.attrs a")
		kinds := element.MayChildrenTexts("#info > span[property*=genre]")
		runtime := element.MayChildText("#info > span[property*=runtime]")
		imdbLink := element.MayChildAttr("#info > a", "href")
		year := element.MayChildText("#content > h1 > span.year")
		picLink := element.ChildAttr("#mainpic > a > img", "src")

		element.ForEach("div#info", func(element *cobweb.HTMLElement) {
			directors := element.ChildrenTexts("span > a[rel*=directedBy]")
			ctx.Item(DoubanItem{
				Title:     title,
				Year:      year,
				PicLink:   picLink,
				Rank:      rank.(string),
				Directors: directors,
				Actors:    actors,
				Kinds:     kinds,
				Runtime:   runtime,
				IMDbLink:  imdbLink,
			})
			ctx.SaveResource(picLink, fmt.Sprintf("%v.%v.%v.cover.jpg", rank, year, title))
		})
	})
}
