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
	if !c.ResponseLegal() {
		logrus.WithFields(logrus.Fields{
			"Context": c.ctx,
		}).Error("非法的 Command")
		return nil
	}

	c.callback(c.ctx)
	c.ctx.Task.CommandComplete(c)
	return c.ctx.Task.GetCommands()
}
