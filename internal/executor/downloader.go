package executor

import (
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"github.com/SolarDomo/Cobweb/pkg/utils"
	"github.com/valyala/fasthttp"
	url2 "net/url"
	"sync"
	"time"
)

type AbsDownloaderFactory interface {
	NewDownloader(downloaderList []*Downloader, maxConcurrentReqCnt int, proxyTopK int) *Downloader
}

var initTime = time.Unix(0, 0)

// 基于 FastHTTP
type Downloader struct {
	client           *fasthttp.Client
	proxy            *models.Proxy
	host2LastReqTime sync.Map
	semaphore        utils.Semaphore

	errCntLocker sync.Mutex
	errCnt       int
}

func (d *Downloader) TryAcquire() bool {
	return d.semaphore.TryAcquire()
}

func (d *Downloader) Release() {
	d.semaphore.Release()
}

func (d *Downloader) GetLastReqTime(url *url2.URL) time.Time {
	if url == nil {
		return initTime
	}

	val, ok := d.host2LastReqTime.Load(url.Hostname())
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

	prevVal, ok := d.host2LastReqTime.Load(cmd.ctx.Request.Host())
	d.host2LastReqTime.Store(cmd.ctx.Request.Host(), time.Now())

	cmd.ctx.RespErr = d.client.DoTimeout(cmd.ctx.Request, cmd.ctx.Response, cmd.ctx.ReqTimeout)

	if !cmd.ResponseLegal() {
		d.host2LastReqTime.Store(cmd.ctx.Request.Host(), time.Now())
	} else {
		if ok {
			d.host2LastReqTime.Store(cmd.ctx.Request.Host(), prevVal)
		} else {
			d.host2LastReqTime.Store(cmd.ctx.Request.Host(), initTime)
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
