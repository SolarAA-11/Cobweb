package douban

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/SolarDomo/Cobweb/internal/executor"
	"github.com/sirupsen/logrus"
	"time"
)

type DoubanTask struct {
	*executor.BaseTask
}

func NewDoubanTask() *DoubanTask {
	t := &DoubanTask{}
	t.BaseTask = executor.NewBastTask(t, time.Second*20)
	for i := 0; i < 10; i++ {
		t.AppendGetCommand(fmt.Sprintf("https://movie.douban.com/top250?start=%d&filter=", i*25), t.scrapeListPage)
	}
	return t
}

func (t *DoubanTask) GetTaskName() string {
	return "DoubanTask"
}

func (t *DoubanTask) scrapeListPage(ctx *executor.Context) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(ctx.Response.Body()))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Context": ctx,
		}).Error("解析文档失败")
	}
	divCount := doc.Find("#content > div > div.article > ol > li > div").Each(func(i int, s *goquery.Selection) {
		detailPageLink, ok := s.Find("div.hd a").Attr("href")
		if !ok {
			logrus.Error("没有详情页面链接")
			return
		}

		t.AppendGetCommand(detailPageLink, t.scrapeDetailPage)

	}).Length()
	if divCount == 0 {
		logrus.WithFields(ctx.LogrusFields()).Error("错误页面")
		ctx.Retry()
	}
}

func (t *DoubanTask) scrapeDetailPage(ctx *executor.Context) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(ctx.Response.Body()))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Context": ctx,
		}).Error("解析文档失败")
	}
	title := doc.Find("#content > h1 > span:nth-child(1)").Text()
	fmt.Println(title)
}
