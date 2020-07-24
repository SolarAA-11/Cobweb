package executor

type Command struct {
	ctx      *Context
	callback OnResponseCallback
}

func (c *Command) GetTaskName() string {
	return c.ctx.BaseTask.GetTaskName()
}

func (c *Command) ResponseLegal() bool {
	return c.ctx.BaseTask.CommandDownloadLegal(c.ctx)
}

func (c *Command) Process() []*Command {
	c.callback(c.ctx)

	if !c.ctx.retry {
		c.ctx.BaseTask.CommandComplete(c)
		return c.ctx.BaseTask.GetCommands()
	} else {
		c.ctx.retry = false
		return append(c.ctx.BaseTask.GetCommands(), c)
	}

}
