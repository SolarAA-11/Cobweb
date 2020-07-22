package executor

import (
	"github.com/SolarDomo/Cobweb/internal/executor/cmddownloader"
	"github.com/SolarDomo/Cobweb/internal/executor/command"
	"github.com/SolarDomo/Cobweb/internal/executor/processor"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

// command.AbsCommand 的 Executor
type CMDExecutor struct {
	downloaderManager *cmddownloader.DownloaderManager
	processor         processor.AbsProcessor

	// processorResultCMDChannel
	// processor 处理完 Command 之后, 可能会产生新的 Command
	// processor 将新的 Command 发送到此 Channel
	// Executor 负责将此 Channel 中 Command 重新处理
	processorResultCMDChannel <-chan command.AbsCommand

	wg sync.WaitGroup
}

func NewCMDExecutor(
	timeout time.Duration,
	downloaderCnt int,
	routineCntPerDownloader int,
	maxDownloaderErrCnt int,
	downloaderFactory cmddownloader.AbsDownloaderFactory,
	processorFactory processor.AbsProcessorFactory,
) *CMDExecutor {
	downloaderManager, downloaderResultCMDChannel := cmddownloader.NewDownloaderManager(
		timeout,
		downloaderCnt,
		routineCntPerDownloader,
		maxDownloaderErrCnt,
		downloaderFactory,
	)
	p, processorResultCMDChannel := processorFactory.NewProcessor(downloaderResultCMDChannel)
	executor := &CMDExecutor{
		downloaderManager:         downloaderManager,
		processor:                 p,
		processorResultCMDChannel: processorResultCMDChannel,
	}
	go executor.workerRoutine()

	return executor
}

func (this *CMDExecutor) AcceptCommand(command command.AbsCommand) bool {
	return this.downloaderManager.AcceptCommand(command)
}

func (this *CMDExecutor) WaitAndStop() {
	this.downloaderManager.WaitAndStop()
	this.processor.WaitAndStop()
	this.wg.Wait()
}

// Processor 处理 Command 之后, 可能会将
// 1. 旧 Command
// 2. 新 Command
// 发送到 processorResultCMDChannel 里
// 此协程需要将 processorResultCMDChannel 里的 Cmd 重写加入到 Executor 处理
func (this *CMDExecutor) workerRoutine() {
	logrus.Info("启动 CMDExecutor Command 协程")
	this.wg.Add(1)

	defer func() {
		logrus.Info("关闭 CMD Executor Command 协程")
		this.wg.Done()
	}()

	for cmd := range this.processorResultCMDChannel {
		this.downloaderManager.AcceptCommand(cmd)
	}
}
