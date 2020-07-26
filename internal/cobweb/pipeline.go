package cobweb

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Item struct {
	task *Task
	data map[string]string
}

// Pipeline send structual data into certain output device
// for example, stdout, mysql, sqlite, redis
type Pipeline interface {
	Pipe(item *Item)
	Close()
}

type StdoutPipeline struct{}

func (p *StdoutPipeline) Pipe(item *Item) {
	j, err := json.MarshalIndent(item.data, "", "\t")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Data": item.data,
		}).Error("StdoutPipeline marshal item data to json failure.")
		return
	}
	fmt.Println(string(j))
}

func (p *StdoutPipeline) Close() {

}

const filePipelineDefaultFolderName = "filepipeline"

type JsonFilePipeline struct {
	filepath string
	locker   sync.Mutex
	items    []*Item
}

func NewJFilePipeline(filepath ...string) *JsonFilePipeline {
	switch len(filepath) {
	case 0:
		return &JsonFilePipeline{}
	case 1:
		return &JsonFilePipeline{filepath: filepath[0]}
	default:
		logrus.WithFields(logrus.Fields{
			"FilePathArg": filepath,
		}).Panic("argument filepath len large than 1")
		return nil
	}
}

func (p *JsonFilePipeline) Pipe(item *Item) {
	p.items = append(p.items, item)
}

func (p *JsonFilePipeline) Close() {
	p.locker.Lock()
	defer p.locker.Unlock()
	if len(p.items) == 0 {
		return
	}

	var filePath string
	if p.filepath == "" {
		fileName := fmt.Sprintf("%v-%v.json", p.items[0].task.Name(), time.Now().Format("2006-01-02 15-04-05"))
		filePath = path.Join("instance", "cobweb-pipe", "json-file-pipeline", fileName)
	} else {
		filePath = path.Join("instance", p.filepath)
	}

	err := os.MkdirAll(path.Dir(filePath), os.ModeDir)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":    err,
			"FilePath": filePath,
			"TaskName": p.items[0].task.Name(),
		}).Error("JsonFilePipeline create directory failure.")
		return
	}

	dataArray := make([]map[string]string, 0, len(p.items))
	for _, item := range p.items {
		if item.data == nil {
			logrus.WithFields(logrus.Fields{}).Fatal("")
		}
		dataArray = append(dataArray, item.data)
	}

	j, err := json.MarshalIndent(dataArray, "", "\t")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":    err,
			"FilePath": filePath,
			"TaskName": p.items[0].task.Name(),
		}).Error("Json marshal item data array failure.")
		return
	}

	err = ioutil.WriteFile(filePath, j, os.ModePerm)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":    err,
			"FilePath": filePath,
			"TaskName": p.items[0].task.Name(),
		}).Error("Write json represent to file failure.")
		return
	}

	logrus.WithFields(logrus.Fields{
		"FilePath": filePath,
		"TaskName": p.items[0].task.Name(),
	}).Info("JsonFilePipeline write finish.")
}
