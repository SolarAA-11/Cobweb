package cmddownloader

import (
	"container/list"
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"github.com/SolarDomo/Cobweb/internal/proxypool/storage"
	"github.com/valyala/fasthttp"
	"time"
)

type FastHTTPFactory struct{}

func (this *FastHTTPFactory) NewDownloader(proxyRefuseList *list.List, proxyCntMin int) AbsCMDDownloader {
	proxy, err := storage.Singleton().GetRandTopKProxy(proxyCntMin * 2)
	if err != nil {
		// todo
		return nil
	}

	for e := proxyRefuseList.Front(); e != nil; e = e.Next() {
		curProxyVal := e.Value
		curProxy, ok := curProxyVal.(*models.Proxy)
		if !ok {
			// todo
			return nil
		}

		if curProxy.Equal(proxy) {
			return this.NewDownloader(proxyRefuseList, proxyCntMin)
		}
	}

	downloader := &fastHTTPDownloader{
		client: &fasthttp.Client{
			Dial: proxy.FastHTTPDialHTTPProxy(),
		},
		proxy: proxy,
	}

	return downloader
}

// 基于 FastHTTP 的 Downloader
type fastHTTPDownloader struct {
	client *fasthttp.Client
	proxy  *models.Proxy
}

func (this *fastHTTPDownloader) GetProxyUsed() *models.Proxy {
	panic("implement me")
}

func (this *fastHTTPDownloader) GetTimeout(url string, timeout time.Duration) (statusCode int, respBody []byte, err error) {
	panic("implement me")
}

func (this *fastHTTPDownloader) IncreaseErrCnt() int {
	panic("implement me")
}

func (this *fastHTTPDownloader) ResetErrCnt(maxErrCnt int) bool {
	panic("implement me")
}

func (this *fastHTTPDownloader) CheckErrCnt(maxErrCnt int) bool {
	panic("implement me")
}
