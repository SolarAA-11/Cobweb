package executor

import "fmt"

type Item struct {
	task *Task
	data map[string]string
}

// Pipeline send structual data into certain output device
// for example, stdout, mysql, sqlite, redis
type Pipeline interface {
	Pipe(item *Item)
}

type StdoutPipeline struct{}

func (p *StdoutPipeline) Pipe(item *Item) {
	fmt.Println(item.data)
}
