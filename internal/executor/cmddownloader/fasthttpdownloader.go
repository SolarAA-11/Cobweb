package cmddownloader

import (
	"container/list"
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"github.com/SolarDomo/Cobweb/internal/proxypool/storage"
	"github.com/valyala/fasthttp"
	"sync"
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

	errCntMut sync.RWMutex
	errCnt    int
}

func (this *fastHTTPDownloader) GetProxyUsed() *models.Proxy {
	return this.proxy
}

func (this *fastHTTPDownloader) GetTimeout(url string, timeout time.Duration) (statusCode int, respBody []byte, err error) {
	return this.client.GetTimeout(nil, url, timeout)
}

func (this *fastHTTPDownloader) IncreaseErrCnt() int {
	this.errCntMut.Lock()
	defer this.errCntMut.Unlock()

	this.errCnt += 1
	return this.errCnt
}

func (this *fastHTTPDownloader) ResetErrCnt(maxErrCnt int) bool {
	this.errCntMut.Lock()
	defer this.errCntMut.Unlock()

	if this.errCnt >= maxErrCnt {
		return false
	} else {
		this.errCnt = 0
		return true
	}
}

func (this *fastHTTPDownloader) CheckErrCnt(maxErrCnt int) bool {
	this.errCntMut.RLock()
	defer this.errCntMut.RUnlock()
	return this.errCnt < maxErrCnt
}

func (this *fastHTTPDownloader) GetDownloaderName() string {
	return "FastHTTPDownloader"
}
