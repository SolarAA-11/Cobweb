package executor

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
