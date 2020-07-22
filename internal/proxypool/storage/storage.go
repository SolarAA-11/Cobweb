package storage

import (
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"sync"
)

type AbsProxyStorage interface {
	GetProxy(proxy *models.Proxy) (*models.Proxy, error)
	HasProxy(proxy *models.Proxy) (bool, error)
	CreateProxy(proxy *models.Proxy) error
	CreateProxyList([]*models.Proxy) int
	ActivateProxy(proxy *models.Proxy) error
	DeactivateProxy(proxy *models.Proxy) error
	GetTopKProxyList(k int) ([]*models.Proxy, error)
	GetRandTopKProxy(k int) (*models.Proxy, error)
	GetAllProxy() ([]*models.Proxy, error)
}

var absStorageSingleton AbsProxyStorage
var once sync.Once

func Singleton() AbsProxyStorage {
	once.Do(func() {
		absStorageSingleton = newDBProxyStorage()
	})
	return absStorageSingleton
}
