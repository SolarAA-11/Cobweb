package main

import (
	"fmt"
	"time"

	"github.com/SolarDomo/Cobweb/example/douban"

	"github.com/SolarDomo/Cobweb/internal/cobweb"
	"github.com/sirupsen/logrus"
)

func main() {
	//logrus.SetReportCaller(true)
	//logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.DebugLevel)
	startTime := time.Now()
	e := cobweb.NewDefaultExecutor()
	t := e.AcceptRule(&douban.DoubanRule{})
	t.Wait()
	fmt.Println("耗时: ", time.Since(startTime))
	e.Stop()
	//e := cobweb.NewDefaultExecutor()
	//pool := cobweb.NewProxyPool(e, time.Second*30, 200)
	//pool.Start()
	//time.Sleep(time.Hour)
	//pool.Stop()
	//e.Stop()
	//fmt.Println("Stopped")
}
