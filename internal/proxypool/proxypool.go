package proxypool

import (
	"encoding/json"
	"github.com/SolarDomo/Cobweb/internal/executor"
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"github.com/SolarDomo/Cobweb/internal/proxypool/storage"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"sync"
	"sync/atomic"
	"time"
)

type ProxyPool struct {
	e         *executor.Executor
	startOnce sync.Once

	workCron *cron.Cron

	checkProxyPoolRunningLocker sync.Mutex
	checkProxyPoolRunning       bool
	proxyReqTimeout             time.Duration
	checkRoutineLimitCh         chan struct{}
	checkProxyWg                sync.WaitGroup

	stopWg   sync.WaitGroup
	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewProxyPool(
	executor *executor.Executor,
	proxyReqTimeout time.Duration,
	checkRoutineMaxCount int,
) *ProxyPool {
	p := &ProxyPool{
		e:                   executor,
		workCron:            cron.New(),
		proxyReqTimeout:     proxyReqTimeout,
		checkRoutineLimitCh: make(chan struct{}, checkRoutineMaxCount),
		stopCh:              make(chan struct{}),
	}

	p.workCron.AddFunc("*/1 * * * *", p.checkProxyPool)
	p.workCron.AddFunc("* */12 * * *", p.fetchProxy)

	return p
}

func (p *ProxyPool) Start() {
	p.startOnce.Do(func() {
		p.workCron.Start()
	})
}

func (p *ProxyPool) Stop() {
	p.stopOnce.Do(func() {
		logrus.Info("ProxyPool stopping...")
		close(p.stopCh)
		p.stopWg.Wait()
	})
	logrus.Info("ProxyPool stopped.")
}

func (p *ProxyPool) fetchProxy() {

}

func (p *ProxyPool) checkProxyPool() {
	p.checkProxyPoolRunningLocker.Lock()
	if p.checkProxyPoolRunning {
		p.checkProxyPoolRunningLocker.Unlock()
		return
	} else {
		p.checkProxyPoolRunning = true
		p.checkProxyPoolRunningLocker.Unlock()
	}

	logrus.Info("Start ProxyPool check")

	p.stopWg.Add(1)
	defer p.stopWg.Done()

	startTime := time.Now()

	proxies, err := storage.Singleton().GetAllProxy()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error": err,
		}).Error("check proxy pool, get all proxy failure.")
		return
	}

	var activatedCounter int32 = 0
	var stop = false
	for _, proxy := range proxies {
		select {
		case <-p.stopCh:
			stop = true
		default:
			p.checkRoutineLimitCh <- struct{}{}
			go p.checkProxy(proxy, &activatedCounter)
		}
		if stop {
			break
		}
	}
	p.checkProxyWg.Wait()

	if stop {
		logrus.WithFields(logrus.Fields{
			"TotalProxyCount":     len(proxies),
			"ActivatedProxyCount": activatedCounter,
			"TimeElapsed":         time.Since(startTime),
		}).Info("ProxyPool check routine has been stopped.")
	} else {
		logrus.WithFields(logrus.Fields{
			"TotalProxyCount":     len(proxies),
			"ActivatedProxyCount": activatedCounter,
			"TimeElapsed":         time.Since(startTime),
		}).Info("Finish ProxyPool Check.")
	}

	p.checkProxyPoolRunningLocker.Lock()
	p.checkProxyPoolRunning = false
	p.checkProxyPoolRunningLocker.Unlock()
}

func (p *ProxyPool) checkProxy(
	proxy *models.Proxy,
	pActivatedCounter *int32,
) {
	p.checkProxyWg.Add(1)
	defer func() {
		<-p.checkRoutineLimitCh
		p.checkProxyWg.Done()
	}()

	if proxy == nil {
		return
	}

	client := &fasthttp.Client{Dial: proxy.FastHTTPDialHTTPProxy()}
	code, body, err := client.GetTimeout(nil, "https://httpbin.org/ip", p.proxyReqTimeout)

	if err == nil && code == 200 {
		val := make(map[string]interface{})
		if err := json.Unmarshal(body, &val); err == nil {
			if originVal, ok := val["origin"]; ok {
				origin, ok := originVal.(string)
				if ok && origin == proxy.Host {
					err := storage.Singleton().ActivateProxy(proxy)
					if err != nil {
						logrus.WithFields(logrus.Fields{
							"Error": err,
							"Proxy": proxy,
						}).Error("activate proxy error")
					} else {
						atomic.AddInt32(pActivatedCounter, 1)
					}
					logrus.WithFields(logrus.Fields{
						"Proxy":    proxy,
						"RespBody": string(body),
					}).Info("Activate Proxy")
					return
				}
			}
		}
	}

	logrus.WithFields(logrus.Fields{
		"Proxy":    proxy,
		"RespBody": string(body),
		"Error":    err,
	}).Info("DeActivate Proxy")

	err = storage.Singleton().DeactivateProxy(proxy)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error": err,
			"Proxy": proxy,
		}).Error("deactivate proxy error")
	}
}
