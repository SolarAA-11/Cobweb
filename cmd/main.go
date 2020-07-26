package main

import (
	"fmt"
	"github.com/SolarDomo/Cobweb/example/douban"
	"github.com/SolarDomo/Cobweb/internal/executor"
	"github.com/sirupsen/logrus"
	"time"
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	startTime := time.Now()
	e := executor.NewDefaultExecutor()
	t := e.AcceptRule(&douban.DoubanRule{})
	t.Wait()
	fmt.Println("耗时: ", time.Since(startTime))
	e.Stop()
}
