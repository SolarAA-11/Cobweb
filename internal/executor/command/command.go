package command

import "sync"

type AbsCommand interface {
	GetTargetURL() string
	SetDownloadResult(respCode int, respBody []byte, respErr error)
	Process() []AbsCommand
	Complete()
	GetCompleteChannel() <-chan struct{}
}

type BaseCommand struct {
	RespCode int
	RespBody []byte
	RespErr  error

	once            sync.Once
	completeChannel chan struct{}
}

func (this *BaseCommand) GetTargetURL() string {
	panic("implement me")
}

func (this *BaseCommand) SetDownloadResult(respCode int, respBody []byte, respErr error) {
	this.RespCode = respCode
	this.RespBody = respBody
	this.RespErr = respErr
}

func (this *BaseCommand) Process() []AbsCommand {
	panic("implement me")
}

func (this *BaseCommand) Complete() {
	close(this.getCompleteChannel())
}

func (this *BaseCommand) getCompleteChannel() chan struct{} {
	this.once.Do(func() {
		this.completeChannel = make(chan struct{})
	})
	return this.completeChannel
}

func (this *BaseCommand) GetCompleteChannel() <-chan struct{} {
	return this.getCompleteChannel()
}
