package storage

import (
	"fmt"
	"github.com/SolarDomo/Cobweb/internal/proxypool/models"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mssql"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"math/rand"

	"github.com/sirupsen/logrus"
)

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

	if !dbConn.HasTable(&models.Proxy{}) {
		err := dbConn.CreateTable(&models.Proxy{}).Error
		if err != nil {
			logrus.WithField("Error", err).Fatal("建立数据库表 Proxy 失败")
		}
	}

	return &dbProxyStorage{dbConn: dbConn}
}

func (this *dbProxyStorage) GetProxy(proxy *models.Proxy) (*models.Proxy, error) {
	if proxy == nil {
		return nil, fmt.Errorf("Proxy is nil")
	}

	proxyInDB := &models.Proxy{}
	err := this.dbConn.Model(proxy).Where(&models.Proxy{Host: proxy.Host, Port: proxy.Port}).Find(proxyInDB).Error
	if err != nil {
		return nil, err
	}

	return proxyInDB, nil
}

func (this *dbProxyStorage) HasProxy(proxy *models.Proxy) (bool, error) {
	if proxy == nil {
		return false, fmt.Errorf("Proxy is nil")
	}

	proxyCount := 0
	err := this.dbConn.Model(proxy).Where(&models.Proxy{Host: proxy.Host, Port: proxy.Port}).Count(&proxyCount).Error
	if err != nil {
		return false, err
	} else {
		return proxyCount != 0, nil
	}
}

func (this *dbProxyStorage) CreateProxy(proxy *models.Proxy) error {
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

func (this *dbProxyStorage) CreateProxyList(proxies []*models.Proxy) int {
	var createCount int = 0
	for _, val := range proxies {
		err := this.CreateProxy(val)
		if err == nil {
			createCount++
		}
	}
	return createCount
}

func (this *dbProxyStorage) ActivateProxy(proxy *models.Proxy) error {
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

func (this *dbProxyStorage) DeactivateProxy(proxy *models.Proxy) error {
	proxyInDB, err := this.GetProxy(proxy)
	if err != nil {
		return err
	}

	proxyInDB.Score--
	err = this.dbConn.Save(proxyInDB).Error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error": err,
			"Proxy": proxyInDB,
		}).Info("降权 Proxy 失败")
		return err
	}

	return nil
}

func (this *dbProxyStorage) GetTopKProxyList(k int) ([]*models.Proxy, error) {
	proxyList := make([]*models.Proxy, 0)

	err := this.dbConn.Model(&models.Proxy{}).Order("score desc").Limit(k).Find(&proxyList).Error
	if err != nil {
		return nil, err
	}

	return proxyList, nil
}

func (this *dbProxyStorage) GetRandTopKProxy(k int) (*models.Proxy, error) {
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

func (this *dbProxyStorage) GetAllProxy() ([]*models.Proxy, error) {
	proxyList := make([]*models.Proxy, 0)
	err := this.dbConn.Model(&models.Proxy{}).Find(&proxyList).Error
	if err != nil {
		return nil, err
	}
	return proxyList, nil
}
