package cobweb

import (
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/SolarDomo/Cobweb/pkg/utils"

	mapset "github.com/deckarep/golang-set"

	"github.com/valyala/fasthttp"

	"github.com/sirupsen/logrus"
)

//
type downloaderManager struct {
	dFactory                  downloaderFactory
	downloaderConcurrentLimit int
	downloaderReqHostInterval time.Duration
	downloaderErrCntLimit     int
	downloaderListLocker      sync.RWMutex
	downloaderList            []*downloader

	inCMDChannel  chan *command
	outCMDChannel chan<- *command

	stopChannel chan struct{}
	stopWg      sync.WaitGroup
	stopOnce    sync.Once
}

func newDownloaderManager(
	dFactory downloaderFactory,
	downloaderCnt int,
	downloaderConcurrentLimit int,
	downloaderErrCntLimit int,
	downloaderReqHostInterval time.Duration,
	inCMDChannel chan *command,
	outCMDChannel chan<- *command,
) *downloaderManager {
	d := &downloaderManager{
		dFactory:                  dFactory,
		downloaderConcurrentLimit: downloaderConcurrentLimit,
		downloaderReqHostInterval: downloaderReqHostInterval,
		downloaderErrCntLimit:     downloaderErrCntLimit,
		inCMDChannel:              inCMDChannel,
		outCMDChannel:             outCMDChannel,
		stopChannel:               make(chan struct{}),
	}

	proxyList := make([]*Proxy, 0, downloaderCnt)
	for i := 0; i < downloaderCnt; i++ {
		curDownloader := d.dFactory.newDownloader(
			proxyList,
			d.downloaderConcurrentLimit,
			d.downloaderReqHostInterval,
		)
		d.downloaderList = append(d.downloaderList, curDownloader)
		proxyList = append(proxyList, curDownloader.proxy())
	}

	for i := 0; i < downloaderCnt*downloaderConcurrentLimit; i++ {
		go d.downloadRoutine(i)
	}

	return d
}

func (d *downloaderManager) stop() {
	d.stopOnce.Do(func() {
		logrus.Info("DownloaderManager stopping.")
		close(d.stopChannel)
	})
	d.stopWg.Wait()
	logrus.Info("DownloaderManager stopped.")
}

// go routine body function
// recv download cmd from inCMDChannel
// download resource acquired by command
// if command think resource download success, put command to outCMDChannel
// otherwise back command to inCMDChannel
func (d *downloaderManager) downloadRoutine(routineID int) {
	d.stopWg.Add(1)
	defer d.stopWg.Done()

	logEntry := logrus.WithFields(logrus.Fields{
		"DownloadRoutineID": routineID,
	})

	var loop = true
	for loop {
		select {
		case cmd, ok := <-d.inCMDChannel:
			if !ok {
				logEntry.Fatal("DownloaderManager's inCMDChannel is closed!!!")
			} else if cmd == nil {
				logEntry.Error("Receive nil command.")
				continue
			}
			success := d.download(cmd)
			if success {
				d.outCMDChannel <- cmd
			} else {
				go d.sleepAndReBackToCMDInChannel(cmd)
			}

		case <-d.stopChannel:
			loop = false
		}
	}

	logEntry.Debug("DownloaderManager download routine stopped.")
}

func (d *downloaderManager) download(cmd *command) bool {
	var (
		bannedCount                    = 0
		acquiredDownloader *downloader = nil
		removedDownloader  *downloader = nil
		removeReason                   = ""
		success                        = false
	)

	//logrus.WithFields(cmd.logrusFields()).Info("start find downloader")
	d.downloaderListLocker.RLock()
	for _, curDownloader := range d.downloaderList {
		err := curDownloader.tryAcquire(cmd, d.downloaderErrCntLimit)
		if err == nil {
			acquiredDownloader = curDownloader
			break
		} else {
			if err == downloadErrHostBanned {
				bannedCount++
				if bannedCount >= len(d.downloaderList)/2 {
					removedDownloader = curDownloader
					removeReason = "banned too much"
				}
			}
		}
	}
	d.downloaderListLocker.RUnlock()
	//logrus.WithFields(cmd.logrusFields()).Info("finish find downloader")

	if acquiredDownloader != nil {
		cmd.downloaderUsed = acquiredDownloader
		err := acquiredDownloader.download(cmd)
		logrus.WithFields(cmd.logrusFields()).WithFields(acquiredDownloader.logrusFields()).Info("finish download")
		if err == nil {
			success = true
		} else {
			success = false
			if err == downloadErrRequestError && !acquiredDownloader.checkMaxErrCnt(d.downloaderErrCntLimit) {
				removedDownloader = acquiredDownloader
				removeReason = "err cnt limit"
			}
		}
	}

	if removedDownloader != nil {
		go d.replace(removedDownloader, removeReason)
	}

	return success
}

func (d *downloaderManager) replace(oldDownloader *downloader, reason string) {
	d.downloaderListLocker.Lock()
	defer d.downloaderListLocker.Unlock()

	var oldDownloaderIndex = -1
	var proxyList = make([]*Proxy, 0, len(d.downloaderList))

	for i, d2 := range d.downloaderList {
		proxyList = append(proxyList, d2.proxy())
		if d2 == oldDownloader {
			oldDownloaderIndex = i
		}
	}
	if oldDownloaderIndex == -1 {
		return
	}

	newDownloader := d.dFactory.newDownloader(proxyList, d.downloaderConcurrentLimit, d.downloaderReqHostInterval)
	if newDownloader != nil {
		// todo
	}
	d.downloaderList = append(d.downloaderList[:oldDownloaderIndex], d.downloaderList[oldDownloaderIndex+1:]...)
	d.downloaderList = append(d.downloaderList, newDownloader)

	err := ProxyStorageSingleton().DeactivateProxy(oldDownloader.proxy())
	if err != nil {
		// todo
	}

	logrus.WithFields(logrus.Fields{
		"OldProxy": oldDownloader.proxy(),
		"NewProxy": newDownloader.proxy(),
		"Reason":   reason,
	}).Info("replace downloader")
}

func (d *downloaderManager) sleepAndReBackToCMDInChannel(cmd *command) {
	//time.Sleep(d.downloaderReqHostInterval)
	d.inCMDChannel <- cmd
}

var (
	downloadErrBadState        = errors.New("downloader's err cnt reaches limit")
	downloadErrReqTooOften     = errors.New("request too often")
	downloadErrHostBanned      = errors.New("request host ban downloader")
	downloadErrConcurrentLimit = errors.New("downloader are running too many requesting")
	downloadErrRequestError    = errors.New("downloader has requested resource, but failed")
)

type downloaderFactory interface {
	newDownloader(proxiesRefuse []*Proxy, concurrentLimit int, hostReqInterval time.Duration) *downloader
}

type NoProxyFastHTTPDownloaderFactory struct {
}

func (f *NoProxyFastHTTPDownloaderFactory) newDownloader(_ []*Proxy, concurrentLimit int, hostReqInterval time.Duration) *downloader {
	return newDownloader(nil, concurrentLimit, hostReqInterval)
}

type ProxyFastHTTPDownloaderFactory struct {
}

func (f *ProxyFastHTTPDownloaderFactory) newDownloader(proxiesRefuse []*Proxy, concurrentLimit int, hostReqInterval time.Duration) *downloader {
	proxy, err := ProxyStorageSingleton().GetRandProxyWithRefuseList(proxiesRefuse)
	if err != nil {
		// todo
	}
	return newDownloader(proxy, concurrentLimit, hostReqInterval)
}

type downloader struct {
	*fastHTTPDownloader
}

func newDownloader(
	proxy *Proxy,
	concurrentLimit int,
	hostReqInterval time.Duration,
) *downloader {
	wrapper := &downloader{
		fastHTTPDownloader: newFastHTTPDownloader(
			proxy,
			concurrentLimit,
			hostReqInterval,
		),
	}
	runtime.SetFinalizer(wrapper, (*downloader).fin)
	return wrapper
}

func (wrapper *downloader) fin() {
	wrapper.fastHTTPDownloader.refreshCron.Stop()
}

type fastHTTPDownloader struct {
	client    *fasthttp.Client
	proxyUsed *Proxy

	reqSemaphore    *utils.Semaphore
	concurrentLimit int

	hostReqInterval     time.Duration
	reqTimeLocker       sync.Mutex
	host2LastReqTimeBak map[string]time.Time
	host2LastReqTime    map[string]time.Time
	bannedHostSet       mapset.Set

	errCntLocker sync.RWMutex
	errCnt       int

	refreshCron *cron.Cron
}

func newFastHTTPDownloader(
	proxy *Proxy,
	concurrentLimit int,
	hostReqInterval time.Duration,
) *fastHTTPDownloader {
	d := &fastHTTPDownloader{
		client:              nil,
		proxyUsed:           proxy,
		reqSemaphore:        utils.NewSemaphore(concurrentLimit),
		concurrentLimit:     concurrentLimit,
		hostReqInterval:     hostReqInterval,
		host2LastReqTimeBak: make(map[string]time.Time),
		host2LastReqTime:    make(map[string]time.Time),
		bannedHostSet:       mapset.NewSet(),
		errCntLocker:        sync.RWMutex{},
		errCnt:              0,
		refreshCron:         cron.New(),
	}

	if d.proxyUsed == nil {
		d.client = &fasthttp.Client{}
	} else {
		d.client = &fasthttp.Client{Dial: d.proxyUsed.FastHTTPDialHTTPProxy()}
	}

	d.refreshCron.AddFunc("*/5 * * * *", d.refreshHostInfo)
	d.refreshCron.Start()

	return d
}

func (d *fastHTTPDownloader) logrusFields() logrus.Fields {
	return logrus.Fields{
		"Proxy":  d.proxyUsed,
		"ErrCnt": d.errCnt,
	}
}

// clear information of host which is not visited long time
func (d *fastHTTPDownloader) refreshHostInfo() {
	d.reqTimeLocker.Lock()
	defer d.reqTimeLocker.Unlock()

	nt := time.Now()
	deletedHostList := make([]string, 0)
	for host, reqTime := range d.host2LastReqTime {
		if reqTime.Add(time.Minute * 5).Before(nt) {
			deletedHostList = append(deletedHostList, host)
		}
	}

	for _, host := range deletedHostList {
		delete(d.host2LastReqTimeBak, host)
		delete(d.host2LastReqTime, host)
		d.bannedHostSet.Remove(host)
	}
}

func (d *fastHTTPDownloader) tryAcquire(cmd *command, errCntLimit int) error {
	if d.errCnt >= errCntLimit {
		return downloadErrBadState
	}

	reqHost := string(cmd.request().Host())
	if d.isBanned(reqHost) {
		return downloadErrHostBanned
	}

	if !d.reqSemaphore.TryAcquire() {
		return downloadErrConcurrentLimit
	}

	d.reqTimeLocker.Lock()
	defer d.reqTimeLocker.Unlock()

	lastReqTime, ok := d.host2LastReqTime[reqHost]
	if !ok {
		lastReqTime = time.Unix(0, 0)
	}

	if lastReqTime.Add(d.hostReqInterval).After(time.Now()) {
		d.reqSemaphore.Release()
		return downloadErrReqTooOften
	}
	d.host2LastReqTimeBak[reqHost] = lastReqTime
	d.host2LastReqTime[reqHost] = time.Now()
	return nil
}

func (d *fastHTTPDownloader) increaseErrCnt() {
	d.errCntLocker.Lock()
	defer d.errCntLocker.Unlock()
	d.errCnt++
}

func (d *fastHTTPDownloader) resetErrCnt() {
	d.errCntLocker.Lock()
	defer d.errCntLocker.Unlock()
	d.errCnt = 0
}

func (d *fastHTTPDownloader) checkMaxErrCnt(maxErrCnt int) bool {
	d.errCntLocker.RLock()
	defer d.errCntLocker.RUnlock()
	return d.errCnt < maxErrCnt
}

func (d *fastHTTPDownloader) proxy() *Proxy {
	return d.proxyUsed
}

func (d *fastHTTPDownloader) beBaned(cmd *command) {
	d.bannedHostSet.Add(string(cmd.request().Host()))
}

func (d *fastHTTPDownloader) isBanned(host string) bool {
	return d.bannedHostSet.Contains(host)
}

func (d *fastHTTPDownloader) download(cmd *command) error {
	defer d.reqSemaphore.Release()

	// start download
	err := d.client.DoTimeout(cmd.request(), cmd.response(), cmd.timeout())
	valid := cmd.downloadFinished(err)

	// command check download result valid
	d.reqTimeLocker.Lock()
	defer d.reqTimeLocker.Unlock()
	reqHost := string(cmd.request().Host())
	if valid {
		// download valid
		d.host2LastReqTime[reqHost] = time.Now()
		d.resetErrCnt()
		return nil
	} else {
		// download invalid
		// back store previous last req time
		d.host2LastReqTime[reqHost] = d.host2LastReqTimeBak[reqHost]
		d.increaseErrCnt()
		return downloadErrRequestError
	}
}
