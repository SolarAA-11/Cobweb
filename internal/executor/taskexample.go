package executor

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"time"
)

type DoubanTask struct {
	*BaseTask
}

func NewDoubanTask() *DoubanTask {
	t := &DoubanTask{}
	t.BaseTask = NewBastTask(t, time.Second*20)
	for i := 0; i < 10; i++ {
		t.AppendCommand(fmt.Sprintf("https://movie.douban.com/top250?start=%d&filter=", i*25), t.scrapeListPage)
	}
	return t
}

func (t *DoubanTask) GetTaskName() string {
	return "DoubanTask"
}

func (t *DoubanTask) scrapeListPage(ctx *Context) {
	if ctx.Response.StatusCode() == 200 {
		//doc, err := goquery.NewDocumentFromReader(bytes.NewReader(ctx.Response.Body()))
		//if err != nil {
		//	logrus.WithFields(logrus.Fields{
		//		"Context": ctx,
		//	}).Error("解析文档失败")
		//}
		//texts := doc.Find("#content > div > div.article > ol > li > div > div.info > div.hd > a > span:nth-child(1)").Map(func(i int, s *goquery.Selection) string {
		//	return s.Text()
		//})
		//for _, text := range texts {
		//	fmt.Println(text)
		//}
		//fmt.Println("--------------------------")

		logrus.Info("Finish one Command")
	}

}
