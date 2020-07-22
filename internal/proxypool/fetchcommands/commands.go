package fetchcommands

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/SolarDomo/Cobweb/internal/executor/command"
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"github.com/SolarDomo/Cobweb/internal/proxypool/storage"
	"github.com/sirupsen/logrus"
	"strings"
)

// 抓取 FreeProxyList 的 Command
type FPLFetchCommand struct {
	command.BaseCommand
}

func (this *FPLFetchCommand) GetTargetURL() string {
	return "https://free-proxy-list.net/"
}

func (this *FPLFetchCommand) Process() []command.AbsCommand {
	if this.RespCode == 200 && this.RespErr == nil {
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(this.RespBody))
		if err != nil {
			// todo
		}

		proxyCreated := 0
		doc.Find("#proxylisttable > tbody > tr").Each(func(i int, selection *goquery.Selection) {
			tdS := selection.Find("td")
			tdTextList := tdS.Map(func(i int, s *goquery.Selection) string {
				return s.Text()
			})
			var anonymite models.AnonymityLevel
			switch strings.ToLower(tdTextList[4]) {
			case "transparent":
				anonymite = models.Transparent
			case "anonymous":
				anonymite = models.Anonymous
			case "elite proxy":
				anonymite = models.Elite
			default:
				anonymite = models.Transparent
			}
			proxy := &models.Proxy{
				Url:       tdTextList[0],
				Port:      tdTextList[1],
				HTTPS:     strings.ToLower(tdTextList[6]) == "yes",
				Anonymity: anonymite,
			}
			err = storage.Singleton().CreateProxy(proxy)
			if err != nil {

			} else {
				proxyCreated++
			}
		})
		this.Complete()
		logrus.WithFields(logrus.Fields{
			"添加代理数量": proxyCreated,
		}).Info("FreeProxyList 代理抓取完成")
		return nil
	} else {
		fmt.Printf("Code: %d, Err: %s\nBody: %s\n", this.RespCode, this.RespErr, string(this.RespBody))
		return []command.AbsCommand{this}
	}
}
