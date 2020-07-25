package main

import (
	"github.com/SolarDomo/Cobweb/example/douban"
	"github.com/SolarDomo/Cobweb/internal/executor"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	e := executor.NewDefaultExecutor()
	t := e.AcceptRule(&douban.DoubanRule{})
	t.Wait()
	e.Stop()
}
