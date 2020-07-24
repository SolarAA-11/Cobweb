package executor

import "github.com/sirupsen/logrus"

type Command struct {
	ctx      *Context
	callback OnResponseCallback
}

func (c *Command) GetTaskName() string {
	return c.ctx.Task.GetTaskName()
}

func (c *Command) ResponseLegal() bool {
	return c.ctx.RespErr == nil
}

func (c *Command) Process() []*Command {
	if c.ResponseLegal() {
		logrus.WithFields(logrus.Fields{
			"Task":    c.ctx.Task.GetTaskName(),
			"Req":     c.ctx.Request.String(),
			"Resp":    c.ctx.Response,
			"RespErr": c.ctx.RespErr,
		}).Error("非法的 Command")
		return nil
	}

	c.callback(c.ctx)
	return c.ctx.Task.GetCommands()
}
