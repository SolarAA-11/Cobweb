package proxypool

import (
	"encoding/json"
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"github.com/SolarDomo/Cobweb/internal/proxypool/storage"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"sync"
	"sync/atomic"
	"time"
)

var once sync.Once
var proxyPoolSingleton *proxyPool

func Singleton() *proxyPool {
	once.Do(func() {
		proxyPoolSingleton = &proxyPool{
			proxyTimeout:            time.Second * 60,
			checkProxyRunning:       false,
			maxCheckProxyRoutineCnt: 500,
			proxyPoolCron:           cron.New(),
		}
	})
	return proxyPoolSingleton
}

type proxyPool struct {
	startOnce     sync.Once
	proxyTimeout  time.Duration
	proxyPoolCron *cron.Cron

	checkProxyRunningMutex  sync.Mutex
	checkProxyRunning       bool
	maxCheckProxyRoutineCnt int
}

func (this *proxyPool) Start() {
	this.startOnce.Do(func() {
		_, err := this.proxyPoolCron.AddFunc("*/1 * * * *", this.checkProxyPool)
		if err != nil {

		}
		this.proxyPoolCron.Start()
	})
}

func (this *proxyPool) checkWorkerRoutine(
	proxy *models.Proxy,
	timeout time.Duration,
	ch chan struct{},
	wg *sync.WaitGroup,
	activateCount *int32,
) {
	defer func() {
		<-ch
		wg.Done()
	}()

	logEntry := logrus.WithField("Proxy", proxy)

	client := &fasthttp.Client{Dial: proxy.FastHTTPDialHTTPProxy()}
	code, body, err := client.GetTimeout(nil, "https://httpbin.org/ip", timeout)
	if err == nil && code == 200 {
		respData := make(map[string]interface{})

		err = json.Unmarshal(body, &respData)
		if err != nil {
			goto deactivate
		}

		originVal, ok := respData["origin"]
		if !ok {
			goto deactivate
		}

		origin, ok := originVal.(string)
		if !ok {
			goto deactivate
		}

		if origin == proxy.Url {
			err = storage.Singleton().ActivateProxy(proxy)
			if err != nil {
				// todo
			} else {
				logEntry.Info("激活代理")
				atomic.AddInt32(activateCount, 1)
			}
			return
		}
	}

deactivate:
	err = storage.Singleton().DeactivateProxy(proxy)
	logEntry.Info("反激活代理")
	if err != nil {
		// todo
	}
}

func (this *proxyPool) checkProxyPool() {

	this.checkProxyRunningMutex.Lock()
	if this.checkProxyRunning {
		this.checkProxyRunningMutex.Unlock()
		return
	} else {
		this.checkProxyRunning = true
		this.checkProxyRunningMutex.Unlock()
	}

	logrus.Info("开始检查代理")
	startTime := time.Now()

	allProxyList, err := storage.Singleton().GetAllProxy()
	if err != nil {
		logrus.WithField("Error", err).Error("检查代理时 获取全部代理失败")
		return
	}

	ch := make(chan struct{}, this.maxCheckProxyRoutineCnt)
	wg := &sync.WaitGroup{}
	var activateCount int32 = 0

	for _, proxy := range allProxyList {

		ch <- struct{}{}
		wg.Add(1)

		go this.checkWorkerRoutine(
			proxy,
			this.proxyTimeout,
			ch,
			wg,
			&activateCount,
		)
	}

	wg.Wait()

	logrus.WithFields(logrus.Fields{
		"激活代理数量": activateCount,
		"耗时":     time.Since(startTime),
	}).Info("完成代理检查")

	this.checkProxyRunningMutex.Lock()
	this.checkProxyRunning = false
	this.checkProxyRunningMutex.Unlock()
}
