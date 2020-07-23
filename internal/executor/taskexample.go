package executor

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/sirupsen/logrus"
	"time"
)

type DoubanTask struct {
	*BaseTask
}

func NewDoubanTask() *DoubanTask {
	t := &DoubanTask{}
	t.BaseTask = NewBastTask(t, time.Second*20)
	t.AppendCommand("https://movie.douban.com/top250?start=0&filter=", t.scrapeListPage)
	return t
}

func (t *DoubanTask) scrapeListPage(ctx *Context) {
	if ctx.Response.StatusCode == 200 {
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(ctx.Response.RespBody))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Error": err,
				"Url":   ctx.Request.Url.String(),
			}).Error("解析文档失败")
		}
		texts := doc.Find("#content > div > div.article > ol > li > div > div.info > div.hd > a > span:nth-child(1)").Map(func(i int, s *goquery.Selection) string {
			return s.Text()
		})
		for _, text := range texts {
			fmt.Println(text)
		}
		fmt.Println("--------------------------")
	}
}
