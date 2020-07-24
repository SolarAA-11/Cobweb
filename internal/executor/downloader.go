package executor

import (
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"github.com/SolarDomo/Cobweb/internal/proxypool/storage"
	"github.com/SolarDomo/Cobweb/pkg/utils"
	"github.com/deckarep/golang-set"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"sync"
	"time"
)

type AbsDownloaderFactory interface {
	NewDownloader(downloaderList []*Downloader, maxConcurrentReqCnt int, proxyTopK int) *Downloader
}

type NoProxyDownloaderFactory struct {
}

func (dFactory *NoProxyDownloaderFactory) NewDownloader(_ []*Downloader, maxConcurrentReqCnt int, _ int) *Downloader {
	return &Downloader{
		client:          &fasthttp.Client{},
		semaphore:       utils.NewSemaphore(maxConcurrentReqCnt),
		requiredHostSet: mapset.NewSet(),
	}
}

type DownloaderFactory struct {
}

func (dFactory *DownloaderFactory) NewDownloader(downloaderList []*Downloader, maxConcurrentReqCnt int, proxyTopK int) *Downloader {
	var proxy *models.Proxy = nil
	for proxy == nil {
		tmpProxy, err := storage.Singleton().GetRandTopKProxy(proxyTopK)
		if err != nil {
			logrus.WithFields(logrus.Fields{}).Fatal("新建 Downloader 时获取代理失败")
		}

		tmpProxyConflict := false
		for _, downloader := range downloaderList {
			if downloader.GetProxyUsed().Equal(tmpProxy) {
				tmpProxyConflict = true
				break
			}
		}
		if !tmpProxyConflict {
			proxy = tmpProxy
		}
	}

	d := &Downloader{
		client:          &fasthttp.Client{Dial: proxy.FastHTTPDialHTTPProxy()},
		proxy:           proxy,
		semaphore:       utils.NewSemaphore(maxConcurrentReqCnt),
		requiredHostSet: mapset.NewSet(),
	}

	return d
}

var initTime = time.Unix(0, 0)

// 基于 FastHTTP
type Downloader struct {
	client           *fasthttp.Client
	proxy            *models.Proxy
	host2LastReqTime sync.Map
	semaphore        *utils.Semaphore

	errCntLocker sync.Mutex
	errCnt       int

	requiredHostSet mapset.Set
}

func (d *Downloader) TryAcquire(req *fasthttp.Request) bool {
	if req == nil {
		logrus.WithField("Proxy", d.proxy).Fatal("req can not be nil")
	}

	if d.requiredHostSet.Contains(string(req.Host())) {
		return false
	} else {
		if d.semaphore.TryAcquire() {
			d.requiredHostSet.Add(string(req.Host()))
			return true
		} else {
			return false
		}
	}
}

func (d *Downloader) Release(req *fasthttp.Request) {
	if req == nil {
		logrus.WithField("Proxy", d.proxy).Fatal("req can not be nil")
	}

	if d.requiredHostSet.Contains(string(req.Host())) {
		d.requiredHostSet.Remove(string(req.Host()))
	} else {
		logrus.WithFields(logrus.Fields{
			"Request": req,
			"Proxy":   d.proxy,
		}).Fatal("Request 的 Host 不在 requiredHostSet 中")
	}
	d.semaphore.Release()
}

func (d *Downloader) GetLastReqTime(req *fasthttp.Request) time.Time {
	if req == nil {
		return initTime
	}

	val, ok := d.host2LastReqTime.Load(string(req.Host()))
	if !ok {
		return initTime
	}

	reqTime, ok := val.(time.Time)
	if !ok {
		return initTime
	}

	return reqTime
}

func (d *Downloader) Do(cmd *Command) {
	if cmd.ctx.Response != nil {
		fasthttp.ReleaseResponse(cmd.ctx.Response)
	}
	cmd.ctx.Response = fasthttp.AcquireResponse()

	prevVal, ok := d.host2LastReqTime.Load(string(cmd.ctx.Request.Host()))
	d.host2LastReqTime.Store(string(cmd.ctx.Request.Host()), time.Now())

	cmd.ctx.RespErr = d.client.DoTimeout(cmd.ctx.Request, cmd.ctx.Response, cmd.ctx.ReqTimeout)

	if cmd.ResponseLegal() {
		d.host2LastReqTime.Store(string(cmd.ctx.Request.Host()), time.Now())
	} else {
		if ok {
			d.host2LastReqTime.Store(string(cmd.ctx.Request.Host()), prevVal)
		} else {
			d.host2LastReqTime.Store(string(cmd.ctx.Request.Host()), initTime)
		}
	}
}

func (d *Downloader) GetProxyUsed() *models.Proxy {
	return d.proxy
}

func (d *Downloader) ResetErrCnt(maxErrCnt int) bool {
	d.errCntLocker.Lock()
	defer d.errCntLocker.Unlock()

	if d.errCnt < maxErrCnt {
		d.errCnt = 0
		return true
	} else {
		return false
	}
}

func (d *Downloader) IncreaseErrCnt(maxErrCnt int) bool {
	d.errCntLocker.Lock()
	defer d.errCntLocker.Unlock()

	d.errCnt++
	return d.errCnt < maxErrCnt
}

func (d *Downloader) CheckErrCnt(maxErrCnt int) bool {
	d.errCntLocker.Lock()
	defer d.errCntLocker.Unlock()

	return d.errCnt < maxErrCnt
}
