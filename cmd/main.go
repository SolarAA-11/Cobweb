package main

import (
	"github.com/SolarDomo/Cobweb/internal/executor"
)

func main() {
	//logrus.SetReportCaller(true)
	//logrus.SetLevel(logrus.DebugLevel)

	t := executor.NewDoubanTask()
	e := executor.NewDefaultNoProxyExecutor()
	e.AcceptTask(t)
	<-t.GetCompleteChan()
	e.WaitAndStop()
}
