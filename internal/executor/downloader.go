package executor

import (
	"errors"
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"github.com/SolarDomo/Cobweb/internal/proxypool/storage"
	"github.com/SolarDomo/Cobweb/pkg/utils"
	mapset "github.com/deckarep/golang-set"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"math"
	"reflect"
	"runtime"
	"sync"
	"time"
)

type downloaderManager struct {
	dFactory downloaderFactory

	downloaderCnt     int
	downloadersLocker sync.RWMutex
	downloaders       []Downloader

	inCMDCh  chan *Command
	outCMDCh chan *Command

	once   sync.Once
	stopCh chan struct{}
	wg     sync.WaitGroup

	maxConcurrentReq int
	maxErrCnt        int

	reqTimeInterval time.Duration
}

func newDownloaderManager(
	dFactory downloaderFactory,
	downloaderCnt int,
	maxConcurrentReq int,
	maxErrCnt int,
	reqTimeInterval time.Duration,
	inCMDCh chan *Command,
	outCMDCh chan *Command,
) *downloaderManager {
	d := &downloaderManager{
		dFactory:         dFactory,
		downloaderCnt:    downloaderCnt,
		maxConcurrentReq: maxConcurrentReq,
		maxErrCnt:        maxErrCnt,
		reqTimeInterval:  reqTimeInterval,
		inCMDCh:          inCMDCh,
		outCMDCh:         outCMDCh,
		stopCh:           make(chan struct{}),
	}

	for i := 0; i < d.downloaderCnt; i++ {
		newDownloader, err := d.dFactory.newDownloader(
			d.downloaders,
			d.maxConcurrentReq,
			int(math.Ceil(float64(d.downloaderCnt)*1.5)),
		)
		if err != nil || newDownloader == nil {
			logrus.WithFields(logrus.Fields{
				"FactoryName":      downloaderFactoryName(dFactory),
				"DownloaderCNT":    downloaderCnt,
				"MaxErrCnt":        maxErrCnt,
				"MaxConcurrentReq": maxConcurrentReq,
				"ReqTimeInterval":  reqTimeInterval,
			}).Panic("Downloader Factory failed to create new downloader.")
		}
		d.downloaders = append(d.downloaders, newDownloader)
		logrus.WithFields(logrus.Fields{
			"Proxy": newDownloader.proxy(),
		}).Info("Downloader Manager create new proxy for init itself.")
	}

	for i := 0; i < downloaderCnt*maxConcurrentReq; i++ {
		go d.downloadRoutine(i)
	}

	return d
}

func (d *downloaderManager) downloadRoutine(id int) {
	d.wg.Add(1)
	defer d.wg.Done()

	logEntry := logrus.WithField("DownloadRoutineID", id)
	logEntry.Debug("Start download routine.")

	var loop = true
	for loop {
		select {
		case cmd, ok := <-d.inCMDCh:
			if !ok {
				logEntry.Panic("Downloader Manager CMD input channel has closed.")
			}

			downloader := d.acquireDownloader(cmd)
			if downloader == nil {
				go func() {
					d.wg.Add(1)
					defer d.wg.Done()
					logEntry.WithFields(cmd.ctx.LogrusFields()).Debug("Can not find proper downloader now. Sleep for next try.")
					time.Sleep(d.reqTimeInterval + time.Second)
					d.inCMDCh <- cmd
				}()
				continue
			}

			downloader.do(cmd)

			if cmd.ctx.downloaderValid() {
				downloader.release(cmd, true)
				if !downloader.resetErrCnt(d.maxErrCnt) {
					d.replaceDownloader(downloader)
				}

				logEntry.WithFields(cmd.ctx.LogrusFields()).WithFields(logrus.Fields{
					"Proxy": downloader.proxy(),
				}).Debug("Finish One Command Download")
				d.outCMDCh <- cmd
			} else {
				downloader.release(cmd, false)
				if !downloader.increaseErrCnt(d.maxErrCnt) {
					d.replaceDownloader(downloader)
				}

				logEntry.WithFields(cmd.ctx.LogrusFields()).WithFields(logrus.Fields{
					"Proxy": downloader.proxy(),
				}).Debug("Failure One Command Download")
				d.inCMDCh <- cmd
			}

		case <-d.stopCh:
			loop = false
		}
	}

	logEntry.Debug("Download routine stopped.")
}

func (d *downloaderManager) replaceDownloaderNoLocker(oldD Downloader, reason string) {
	dIndex := -1
	for i, d2 := range d.downloaders {
		if d2 == oldD {
			dIndex = i
			break
		}
	}
	if dIndex == -1 {
		return
	}

	d.downloaders = append(d.downloaders[:dIndex], d.downloaders[dIndex+1:]...)
	err := storage.Singleton().DeactivateProxy(oldD.proxy())
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error": err,
			"Proxy": oldD.proxy(),
		}).Error("Deactivate proxy failed.")
	}

	newD, err := d.dFactory.newDownloader(d.downloaders, d.maxConcurrentReq, int(math.Ceil(float64(d.downloaderCnt)*1.5)))
	if newD == nil || err != nil {
		logrus.WithFields(logrus.Fields{
			"OldProxy":     oldD.proxy(),
			"FactoryError": err,
		}).Panic("replace old downloader fail because factory can't create new old.")
	}

	d.downloaders = append(d.downloaders, newD)
	logrus.WithFields(logrus.Fields{
		"OldProxy": oldD.proxy(),
		"NewProxy": newD.proxy(),
		"Reason":   reason,
	}).Info("Replace old proxy.")
}

func (d *downloaderManager) replaceDownloader(oldD Downloader) {
	d.downloadersLocker.Lock()
	defer d.downloadersLocker.Unlock()
	d.replaceDownloaderNoLocker(oldD, "ErrCnt Limit")
}

func (d *downloaderManager) acquireDownloader(cmd *Command) Downloader {
	d.downloadersLocker.Lock()
	defer d.downloadersLocker.Unlock()

	allBannedRefuse := true
	for _, curDownloader := range d.downloaders {
		err := curDownloader.tryAcquire(cmd, d.reqTimeInterval)
		if err == nil {
			return curDownloader
		} else if err != ERR_BANNED_HOST {
			allBannedRefuse = false
		}
	}

	if allBannedRefuse {
		// all downloader refuse cmd because of banned host
		d.replaceDownloaderNoLocker(d.downloaders[len(d.downloaders)-1], "Banned Too Many")
	}

	return nil
}

func (d *downloaderManager) Stop() {
	logrus.Info("DownloaderManager stopping...")
	d.once.Do(func() {
		close(d.stopCh)
	})
	d.wg.Wait()
	logrus.Info("DownloaderManager stopped.")
}

type downloaderFactory interface {
	newDownloader(downloaders []Downloader, maxConcurrentReq int, k int) (Downloader, error)
}

func downloaderFactoryName(factory downloaderFactory) string {
	fVal := reflect.ValueOf(factory)
	if fVal.Kind() == reflect.Struct {
		return fVal.Type().Name()
	}

	if fVal.Kind() == reflect.Ptr && fVal.Elem().Kind() == reflect.Struct {
		return fVal.Elem().Type().Name()
	}

	panic("can't get downloader factory name")
}

// create downloader which use proxy
type proxyDownloaderFactory struct {
}

func (d *proxyDownloaderFactory) newDownloader(downloaders []Downloader, maxConcurrentReq int, k int) (Downloader, error) {
	var proxy *models.Proxy = nil
	for {
		tempProxy, err := storage.Singleton().GetRandTopKProxy(k)
		if err != nil {
			return nil, err
		}

		if tempProxy == nil {
			return nil, errors.New("proxy storage generate nil proxy")
		}

		var conflict = false
		for _, d2 := range downloaders {
			if d2.proxy().Equal(tempProxy) {
				conflict = true
				break
			}
		}
		if !conflict {
			proxy = tempProxy
			break
		}
	}

	return newDownloaderWrapper(proxy, maxConcurrentReq), nil
}

// create downloader which does not use proxy
type noProxyDownloaderFactory struct {
}

func (n *noProxyDownloaderFactory) newDownloader(_ []Downloader, maxConcurrentReq int, _ int) (Downloader, error) {
	return newDownloaderWrapper(nil, maxConcurrentReq), nil
}

type Downloader interface {
	do(cmd *Command)
	proxy() *models.Proxy
	increaseErrCnt(maxErrCnt int) bool
	resetErrCnt(maxErrCnt int) bool
	tryAcquire(cmd *Command, interval time.Duration) error
	release(cmd *Command, respValid bool)
	lastReqTime(cmd *Command) time.Time
	banned(ctx *Context)
}

type downloaderWrapper struct {
	*downloader
}

func newDownloaderWrapper(
	proxy *models.Proxy,
	maxConcurrentRequestCnt int,
) *downloaderWrapper {
	d := &downloaderWrapper{newDownloader(proxy, maxConcurrentRequestCnt)}
	runtime.SetFinalizer(d, (*downloaderWrapper).finalizer)
	return d
}

func (d *downloaderWrapper) finalizer() {
	d.downloader.clearCron.Stop()
}

type downloader struct {
	client                    *fasthttp.Client
	proxyUsed                 *models.Proxy
	errCntLocker              sync.Mutex
	errCnt                    int
	maxConcurrentReqSemaphore *utils.Semaphore

	hostReqTimeLocker sync.Mutex
	hostReqTime       map[string]time.Time
	hostReqTimeBackup map[string]time.Time

	bannedHostSet mapset.Set

	clearCron *cron.Cron
}

func newDownloader(
	proxy *models.Proxy,
	maxConcurrentRequestCnt int,
) *downloader {
	var client *fasthttp.Client
	if proxy == nil {
		client = &fasthttp.Client{}
	} else {
		client = &fasthttp.Client{Dial: proxy.FastHTTPDialHTTPProxy()}
	}

	d := &downloader{
		client:                    client,
		proxyUsed:                 proxy,
		maxConcurrentReqSemaphore: utils.NewSemaphore(maxConcurrentRequestCnt),
		hostReqTime:               make(map[string]time.Time),
		hostReqTimeBackup:         make(map[string]time.Time),
		bannedHostSet:             mapset.NewSet(),
		clearCron:                 cron.New(),
	}

	d.clearCron.AddFunc("*/10 * * * *", d.clearUnusedHostInfo)
	d.clearCron.Start()
	return d
}

func (d *downloader) clearUnusedHostInfo() {
	d.hostReqTimeLocker.Lock()
	defer d.hostReqTimeLocker.Unlock()

	for host, reqTime := range d.hostReqTime {
		if time.Now().Sub(reqTime).Minutes() >= 10.0 {
			delete(d.hostReqTime, host)
			delete(d.hostReqTimeBackup, host)
			d.bannedHostSet.Remove(host)
			logrus.WithFields(logrus.Fields{
				"Host":            host,
				"LastRequestTime": reqTime,
				"Proxy":           d.proxy(),
			}).Info("Long time no request, delete host request time info.")
		}
	}
}

func (d *downloader) do(cmd *Command) {
	cmd.ctx.downloader = d
	cmd.ctx.Response.Reset()
	cmd.ctx.RespErr = d.client.DoTimeout(cmd.ctx.Request, cmd.ctx.Response, cmd.ctx.task.requestTimeout)
}

func (d *downloader) proxy() *models.Proxy {
	return d.proxyUsed
}

func (d *downloader) increaseErrCnt(maxErrCnt int) bool {
	d.errCntLocker.Lock()
	defer d.errCntLocker.Unlock()
	d.errCnt++
	return d.errCnt < maxErrCnt
}

func (d *downloader) resetErrCnt(maxErrCnt int) bool {
	d.errCntLocker.Lock()
	defer d.errCntLocker.Unlock()
	if d.errCnt < maxErrCnt {
		d.errCnt = 0
		return true
	} else {
		return false
	}
}

func (d *downloader) lastReqTime(cmd *Command) time.Time {
	d.hostReqTimeLocker.Lock()
	defer d.hostReqTimeLocker.Unlock()
	lastReqTime, ok := d.hostReqTime[string(cmd.ctx.Request.Host())]
	if ok {
		return lastReqTime
	} else {
		return time.Unix(0, 0)
	}
}

func (d *downloader) tryAcquire(cmd *Command, interval time.Duration) error {
	if d.bannedHostSet.Contains(string(cmd.ctx.Request.Host())) {
		return ERR_BANNED_HOST
	}

	if !d.maxConcurrentReqSemaphore.TryAcquire() {
		return ERR_CONCURRENT_REQUEST_LIMIT
	}

	d.hostReqTimeLocker.Lock()
	defer d.hostReqTimeLocker.Unlock()
	lastReqTime, ok := d.hostReqTime[string(cmd.ctx.Request.Host())]
	if !ok {
		d.hostReqTimeBackup[string(cmd.ctx.Request.Host())] = time.Unix(0, 0)
		d.hostReqTime[string(cmd.ctx.Request.Host())] = time.Now()
		return nil
	} else {
		if lastReqTime.Add(interval).Before(time.Now()) {
			d.hostReqTimeBackup[string(cmd.ctx.Request.Host())] = lastReqTime
			d.hostReqTime[string(cmd.ctx.Request.Host())] = time.Now()
			return nil
		} else {
			d.maxConcurrentReqSemaphore.Release()
			return ERR_REQUEST_INTERVAL_LIMIT
		}
	}
}

func (d *downloader) banned(ctx *Context) {
	d.bannedHostSet.Add(string(ctx.Request.Host()))
}

func (d *downloader) release(cmd *Command, respValid bool) {
	defer d.maxConcurrentReqSemaphore.Release()

	d.hostReqTimeLocker.Lock()
	defer d.hostReqTimeLocker.Unlock()
	if respValid {
		d.hostReqTime[string(cmd.ctx.Request.Host())] = time.Now()
	} else {
		d.hostReqTime[string(cmd.ctx.Request.Host())] = d.hostReqTimeBackup[string(cmd.ctx.Request.Host())]
	}
}
