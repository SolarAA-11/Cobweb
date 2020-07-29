package cobweb

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"sync"

	"github.com/sirupsen/logrus"
)

type Pipeline interface {
	Pipe(info *itemInfo)
	Close()
}

type itemInfo struct {
	ctx  *Context
	item interface{}
}

func (i *itemInfo) logrusFields() logrus.Fields {
	return i.ctx.logrusFields()
}

type pipeliner struct {
	inItemInfoChannel <-chan *itemInfo

	stopOnce    sync.Once
	stopWg      sync.WaitGroup
	stopChannel chan struct{}
}

func newPipeliner(inItemInfoChannel <-chan *itemInfo) *pipeliner {
	p := &pipeliner{
		inItemInfoChannel: inItemInfoChannel,
		stopOnce:          sync.Once{},
		stopWg:            sync.WaitGroup{},
		stopChannel:       make(chan struct{}),
	}

	for i := 0; i < runtime.NumCPU()*2; i++ {
		go p.pipelineRoutine(i)
	}

	return p
}

func (p *pipeliner) stop() {
	p.stopOnce.Do(func() {
		logrus.Info("Pipeline is stopping...")
		close(p.stopChannel)
	})
	p.stopWg.Wait()
	logrus.Info("Pipeline has stopped...")
}

// get itemInfo from inItemInfoChannel pipe it proper Pipeline indicated by command
func (p *pipeliner) pipelineRoutine(routineID int) {
	p.stopWg.Add(1)
	logEntry := logrus.WithField("RoutineID", routineID)
	defer func() {
		p.stopWg.Done()
	}()

	var loop = true
	for loop {
		select {
		case info, ok := <-p.inItemInfoChannel:
			if !ok {
				logEntry.Fatal("InItemInfoChannel has closed, bad usage.")
			} else if info == nil {
				logEntry.Error("ItemInfo is nil.")
				continue
			}
			info.ctx.cmd.pipe(info)
		case <-p.stopChannel:
			loop = false
		}
	}
}

type JsonStdoutPipeline struct {
}

func (p *JsonStdoutPipeline) Pipe(info *itemInfo) {
	//jd, err := json.MarshalIndent(info.item, "", "")
	jd, err := json.Marshal(info.item)
	if err != nil {

	}
	fmt.Println(string(jd))
}

func (p *JsonStdoutPipeline) Close() {

}

type JsonFilePipeline struct {
	infos []*itemInfo
}

func (p *JsonFilePipeline) Pipe(info *itemInfo) {
	p.infos = append(p.infos, info)
}

func (p *JsonFilePipeline) Close() {
	if len(p.infos) == 0 {
		return
	}

	jsonFilePath := path.Join(p.infos[0].ctx.cmd.task.folderPath(), "items.json")
	err := os.MkdirAll(path.Dir(jsonFilePath), os.ModeDir)
	if err != nil {
		// todo
	}

	items := make([]interface{}, 0, len(p.infos))
	for _, info := range p.infos {
		items = append(items, info.item)
	}
	j, err := json.MarshalIndent(items, "", "\t")
	if err != nil {
		// todo
	}

	err = ioutil.WriteFile(jsonFilePath, j, os.ModePerm)
	if err != nil {
		// todo
	}

	logrus.WithField("JsonFilePath", jsonFilePath).Info("JsonFilePipeline saved items.")
}
