package cmddownloader

import (
	"container/list"
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"time"
)

// 一个 Downloader 基于一个 Proxy 或
type AbsCMDDownloader interface {
	GetDownloaderName() string
	GetProxyUsed() *models.Proxy
	GetTimeout(url string, timeout time.Duration) (statusCode int, respBody []byte, err error)
	IncreaseErrCnt() int
	ResetErrCnt(maxErrCnt int) bool
	CheckErrCnt(maxErrCnt int) bool
}

// DownloaderFactory
type AbsDownloaderFactory interface {
	// 生成新的 Downloader
	// proxyRefuseList: 不能使用的 Proxy 列表
	// proxyCntMin: 最小代理数量 比如使用的 随机 K 值 必须大于等于此参数
	NewDownloader(proxyRefuseList *list.List, proxyCntMin int) AbsCMDDownloader
}
