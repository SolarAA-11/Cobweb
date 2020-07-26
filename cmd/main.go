package main

import (
	"fmt"
	"time"

	"github.com/SolarDomo/Cobweb/example/douban"
	"github.com/SolarDomo/Cobweb/internal/cobweb"
)

func main() {
	//logrus.SetLevel(logrus.DebugLevel)
	startTime := time.Now()
	e := cobweb.NewDefaultExecutor()
	t := e.AcceptRule(&douban.DoubanRule{})
	t.Wait()
	fmt.Println("耗时: ", time.Since(startTime))
	e.Stop()
	//e := cobweb.NewDefaultExecutor()
	//pool := proxypool.NewProxyPool(e, time.Second*30, 10)
	//pool.Start()
	//time.Sleep(time.Minute * 2)
	//pool.Stop()
	//e.Stop()
	//fmt.Println("Stopped")
}
