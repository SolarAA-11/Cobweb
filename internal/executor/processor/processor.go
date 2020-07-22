package processor

import "github.com/SolarDomo/Cobweb/internal/executor/command"

type AbsProcessor interface {
	GetProcessorName() string
	WaitAndStop()
}

type AbsProcessorFactory interface {
	NewProcessor(commandChannel <-chan command.AbsCommand) (AbsProcessor, <-chan command.AbsCommand)
}
