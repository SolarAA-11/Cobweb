package executor

import (
	"github.com/SolarDomo/Cobweb/pkg/utils"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// BaseTask 的 Command 下载完成后的回调函数
type OnResponseCallback func(ctx *Context)

//
type H map[string]interface{}

type AbsTask interface {
	GetTaskName() string
	GetCommands() []*Command
	GetCompleteChan() <-chan struct{}
	CommandDownloadLegal(ctx *Context) bool
	IncreaseErrCnt()
	GetErrCnt() int
}

type BaseTask struct {
	taskName string

	timeout time.Duration

	readyCMDsLocker sync.Mutex
	readyCMDs       []*Command

	runningCMDsLocker sync.Mutex
	runningCMDs       []*Command

	once         sync.Once
	completeChan chan struct{}

	// 所有 Command 的 ErrCnt 和
	errCntLocker sync.Mutex
	errCnt       int
}

func NewBastTask(
	taskName string,
	timeout time.Duration,
) *BaseTask {
	bt := &BaseTask{
		taskName: taskName,
		timeout:  timeout,
	}
	return bt
}

func (b *BaseTask) GetTaskName() string {
	return b.taskName
}

func (b *BaseTask) GetCommands() []*Command {
	b.readyCMDsLocker.Lock()
	cmds := append([]*Command{}, b.readyCMDs...)
	b.readyCMDs = b.readyCMDs[0:0]
	b.readyCMDsLocker.Unlock()

	b.runningCMDsLocker.Lock()
	b.runningCMDs = append(b.runningCMDs, cmds...)
	b.runningCMDsLocker.Unlock()

	return cmds
}

func (b *BaseTask) getCompleteChan() chan struct{} {
	b.once.Do(func() {
		b.completeChan = make(chan struct{})
	})
	return b.completeChan
}

func (b *BaseTask) GetCompleteChan() <-chan struct{} {
	return b.getCompleteChan()
}

func (b *BaseTask) appendCommand(cmd *Command) {
	b.readyCMDsLocker.Lock()
	defer b.readyCMDsLocker.Unlock()
	b.readyCMDs = append(b.readyCMDs, cmd)
}

func (b *BaseTask) NewCommand(
	method string,
	uri string,
	callback OnResponseCallback,
	extraData ...H,
) *Command {
	req := fasthttp.AcquireRequest()
	req.Header.SetMethod(strings.ToUpper(method))
	req.SetRequestURI(uri)

	ctx := &Context{
		BaseTask:   b,
		Request:    req,
		Response:   nil,
		ReqTimeout: b.timeout,
		RespErr:    nil,
	}

	if len(extraData) == 1 {
		for key, val := range extraData[0] {
			ctx.Set(key, val)
		}
	} else if len(extraData) > 1 {
		logrus.Fatal("extraData 参数最多一个")
	}

	cmd := &Command{
		ctx:      ctx,
		callback: callback,
	}

	return cmd
}

func (b *BaseTask) NewGetCommand(
	uri string,
	callback OnResponseCallback,
	extraData ...H,
) *Command {
	return b.NewCommand("get", uri, callback, extraData...)
}

func (b *BaseTask) AppendGetCommand(
	url string,
	callback OnResponseCallback,
	extraData ...H,
) {
	cmd := b.NewGetCommand(url, callback, extraData...)
	if cmd == nil {
		return
	}
	b.appendCommand(cmd)
}

func (b *BaseTask) AppendSaveCommand(url string, name ...string) {
	if len(name) > 1 {
		logrus.WithField("name", name).Fatal("name 参数个数不能大于 1")
	}

	var fName string
	if len(name) == 0 {
		fName = path.Join("instance", utils.GetURLSaveFileName(url))
	} else {
		name[0] = strings.TrimSpace(name[0])
		if len(name[0]) == 0 {
			fName = path.Join("instance", utils.GetURLSaveFileName(url))
		} else {
			fName = path.Join("instance", name[0])
		}
	}

	b.AppendGetCommand(url, b.saveCommandCallback, H{
		"FileName": fName,
	})
}

func (b *BaseTask) saveCommandCallback(ctx *Context) {
	if b.CommandDownloadLegal(ctx) {
		// 使用 BaseTask 时, 可能会覆盖 CommandDownloadLegal 方法
		// 所以在这里使用 BaseTask 重新验证一遍
		logEntry := logrus.WithFields(ctx.LogrusFields())
		val, ok := ctx.Get("FileName")
		if !ok {
			logEntry.Error("缺少 FileName 字段")
			return
		}

		fileName, ok := val.(string)
		if !ok {
			logEntry.Error("缺少 FileName 字段")
			return
		}

		err := os.MkdirAll(path.Dir(fileName), os.ModeDir)
		if err != nil {
			logEntry.WithFields(logrus.Fields{
				"Error":    err,
				"FileName": fileName,
			}).Error("创建文件夹失败")
			return
		}

		f, err := os.Create(fileName)
		if err != nil {
			logEntry.WithFields(logrus.Fields{
				"Error":    err,
				"FileName": fileName,
			}).Error("创建文件失败")
			return
		}

		wLen, err := f.Write(ctx.Response.Body())
		if err != nil {
			logEntry.WithFields(logrus.Fields{
				"Error":    err,
				"FileName": fileName,
			}).Error("写入文件失败")
		} else if wLen != len(ctx.Response.Body()) {
			logEntry.WithFields(logrus.Fields{
				"FileName":    fileName,
				"WriteToFile": wLen,
				"RespBodyLen": len(ctx.Response.Body()),
			}).Error("写入文件字节数不一致")
		}

		logEntry.WithFields(logrus.Fields{
			"FileName": fileName,
			"WriteLen": wLen,
		}).Info("保存到文件")
		f.Close()
	}
}

func (b *BaseTask) CommandComplete(command *Command) {
	b.runningCMDsLocker.Lock()
	defer b.runningCMDsLocker.Unlock()

	for i := range b.runningCMDs {
		if b.runningCMDs[i] == command {
			b.runningCMDs = append(b.runningCMDs[:i], b.runningCMDs[i+1:]...)
			break
		}
	}

	b.readyCMDsLocker.Lock()
	defer b.readyCMDsLocker.Unlock()
	if len(b.runningCMDs)+len(b.readyCMDs) == 0 {
		close(b.getCompleteChan())
		logrus.WithFields(logrus.Fields{
			"Task": b.GetTaskName(),
		}).Info("Task 完成")
	}
}

func (b *BaseTask) CommandDownloadLegal(ctx *Context) bool {
	if ctx == nil || ctx.Response == nil {
		return false
	}
	return ctx.RespErr == nil && ctx.Response.StatusCode() == 200
}

func (b *BaseTask) IncreaseErrCnt() {
	b.errCntLocker.Lock()
	defer b.errCntLocker.Unlock()
	b.errCnt++
}

func (b *BaseTask) GetErrCnt() int {
	b.errCntLocker.Lock()
	defer b.errCntLocker.Unlock()
	return b.errCnt
}

func (b *BaseTask) GetBaseTask() *BaseTask {
	return b
}
