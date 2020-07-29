package cobweb

import (
	"fmt"
	"math/rand"
	"sync"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mssql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/sirupsen/logrus"
)

type AbsProxyStorage interface {
	GetProxy(proxy *Proxy) (*Proxy, error)
	HasProxy(proxy *Proxy) (bool, error)
	CreateProxy(proxy *Proxy) error
	CreateProxyList([]*Proxy) int
	ActivateProxy(proxy *Proxy) error
	DeactivateProxy(proxy *Proxy) error
	GetTopKProxyList(k int) ([]*Proxy, error)
	GetRandTopKProxy(k int) (*Proxy, error)
	GetAllProxy() ([]*Proxy, error)
	GetProxyListWithRefuseList(refuseList []*Proxy, count int) ([]*Proxy, error)
	GetRandProxyWithRefuseList([]*Proxy) (*Proxy, error)
}

var absStorageSingleton AbsProxyStorage
var dbStorageOnce sync.Once

func ProxyStorageSingleton() AbsProxyStorage {
	dbStorageOnce.Do(func() {
		absStorageSingleton = newDBProxyStorage()
	})
	return absStorageSingleton
}

type dbProxyStorage struct {
	dbConn *gorm.DB
}

func newDBProxyStorage() *dbProxyStorage {
	dialect, connURL := "sqlite3", "./instance/data.db3"
	dbConn, err := gorm.Open(dialect, connURL)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":     err,
			"Dialect":   dialect,
			"DBConnURL": connURL,
		}).Fatal("建立数据库连接失败")
		return nil
	}

	if !dbConn.HasTable(&Proxy{}) {
		err := dbConn.CreateTable(&Proxy{}).Error
		if err != nil {
			logrus.WithField("Error", err).Fatal("建立数据库表 Proxy 失败")
		}
	}

	return &dbProxyStorage{dbConn: dbConn}
}

func (this *dbProxyStorage) GetProxy(proxy *Proxy) (*Proxy, error) {
	if proxy == nil {
		return nil, fmt.Errorf("Proxy is nil")
	}

	proxyInDB := &Proxy{}
	err := this.dbConn.Model(proxy).Where(&Proxy{Host: proxy.Host, Port: proxy.Port}).Find(proxyInDB).Error
	if err != nil {
		return nil, err
	}

	return proxyInDB, nil
}

func (this *dbProxyStorage) HasProxy(proxy *Proxy) (bool, error) {
	if proxy == nil {
		return false, fmt.Errorf("Proxy is nil")
	}

	proxyCount := 0
	err := this.dbConn.Model(proxy).Where(&Proxy{Host: proxy.Host, Port: proxy.Port}).Count(&proxyCount).Error
	if err != nil {
		return false, err
	} else {
		return proxyCount != 0, nil
	}
}

func (this *dbProxyStorage) CreateProxy(proxy *Proxy) error {
	if proxy == nil {
		return fmt.Errorf("Proxy is nil")
	}

	exists, err := this.HasProxy(proxy)
	if err != nil {
		return err
	} else if exists {
		return fmt.Errorf("代理已经存在")
	}

	proxy.Score = 80
	return this.dbConn.Save(proxy).Error
}

func (this *dbProxyStorage) CreateProxyList(proxies []*Proxy) int {
	var createCount int = 0
	for _, val := range proxies {
		err := this.CreateProxy(val)
		if err == nil {
			createCount++
		}
	}
	return createCount
}

func (this *dbProxyStorage) ActivateProxy(proxy *Proxy) error {
	proxyInDB, err := this.GetProxy(proxy)
	if err != nil {
		return err
	}

	proxyInDB.Score = 100
	err = this.dbConn.Save(proxyInDB).Error
	if err != nil {
		return err
	}

	return nil
}

func (this *dbProxyStorage) DeactivateProxy(proxy *Proxy) error {
	proxyInDB, err := this.GetProxy(proxy)
	if err != nil {
		return err
	}
	err = this.dbConn.Model(proxyInDB).Where("host", proxy.Host).Update("score", gorm.Expr("score - 1")).Error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error": err,
			"Proxy": proxyInDB,
		}).Info("降权 Proxy 失败")
		return err
	}

	return nil
}

func (this *dbProxyStorage) GetTopKProxyList(k int) ([]*Proxy, error) {
	proxyList := make([]*Proxy, 0)

	err := this.dbConn.Model(&Proxy{}).Order("score desc").Limit(k).Find(&proxyList).Error
	if err != nil {
		return nil, err
	}

	return proxyList, nil
}

func (this *dbProxyStorage) GetRandTopKProxy(k int) (*Proxy, error) {
	proxyList, err := this.GetTopKProxyList(k)
	if err != nil {
		return nil, err
	}

	if len(proxyList) == 0 {
		return nil, nil
	} else {
		return proxyList[rand.Intn(k)], nil
	}
}

func (this *dbProxyStorage) GetAllProxy() ([]*Proxy, error) {
	proxyList := make([]*Proxy, 0)
	err := this.dbConn.Model(&Proxy{}).Find(&proxyList).Error
	if err != nil {
		return nil, err
	}
	return proxyList, nil
}

func (this *dbProxyStorage) GetProxyListWithRefuseList(refuseList []*Proxy, count int) ([]*Proxy, error) {
	conn := this.dbConn.Model(&Proxy{})
	if len(refuseList) != 0 {
		hostList := make([]string, 0, len(refuseList))
		for _, p := range refuseList {
			hostList = append(hostList, p.Host)
		}
		conn = conn.Not("host", hostList)
	}

	proxyList := make([]*Proxy, 0, count)
	err := conn.Order("score desc").Limit(count).Find(&proxyList).Error
	return proxyList, err
}

func (this *dbProxyStorage) GetRandProxyWithRefuseList(refuseList []*Proxy) (*Proxy, error) {
	conn := this.dbConn.Model(&Proxy{})
	if len(refuseList) != 0 {
		hostList := make([]string, 0, len(refuseList))
		for _, proxy := range refuseList {
			hostList = append(hostList, proxy.Host)
		}
		conn = conn.Not("host", hostList)
	}

	proxyList := make([]*Proxy, 0)
	err := conn.Order("score desc").Limit(10).Find(&proxyList).Error
	randIndex := rand.Intn(len(proxyList))
	return proxyList[randIndex], err
}
