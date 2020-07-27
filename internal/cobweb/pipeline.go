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

type ItemInfo struct {
	task *Task
	item interface{}
}

// Pipeline send structual data into certain output device
// for example, stdout, mysql, sqlite, redis
type Pipeline interface {
	Pipe(info *ItemInfo)
	Close()
}

type StdoutPipeline struct{}

func (p *StdoutPipeline) Pipe(info *ItemInfo) {
	j, err := json.MarshalIndent(info.item, "", "\t")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Item": info.item,
		}).Error("StdoutPipeline marshal item data to json failure.")
		return
	}
	fmt.Println(string(j))
}

func (p *StdoutPipeline) Close() {

}

const filePipelineDefaultFolderName = "filepipeline"

type JsonFilePipeline struct {
	filepath  string
	locker    sync.Mutex
	itemInfos []*ItemInfo
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

func (p *JsonFilePipeline) Pipe(info *ItemInfo) {
	p.locker.Lock()
	defer p.locker.Unlock()
	p.itemInfos = append(p.itemInfos, info)
}

func (p *JsonFilePipeline) Close() {
	p.locker.Lock()
	defer p.locker.Unlock()
	if len(p.itemInfos) == 0 {
		return
	}

	var filePath string
	if p.filepath == "" {
		fileName := fmt.Sprintf("%v-%v.json", p.itemInfos[0].task.Name(), time.Now().Format("2006-01-02 15-04-05"))
		filePath = path.Join("instance", "cobweb-pipe", "json-file-pipeline", fileName)
	} else {
		filePath = path.Join("instance", p.filepath)
	}

	err := os.MkdirAll(path.Dir(filePath), os.ModeDir)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":    err,
			"FilePath": filePath,
			"TaskName": p.itemInfos[0].task.Name(),
		}).Error("JsonFilePipeline create directory failure.")
		return
	}

	dataArray := make([]interface{}, 0, len(p.itemInfos))
	for _, info := range p.itemInfos {
		if info.item == nil {
			logrus.WithFields(logrus.Fields{}).Fatal("")
		}
		dataArray = append(dataArray, info.item)
	}

	j, err := json.MarshalIndent(dataArray, "", "\t")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":    err,
			"FilePath": filePath,
			"TaskName": p.itemInfos[0].task.Name(),
		}).Error("Json marshal item data array failure.")
		return
	}

	err = ioutil.WriteFile(filePath, j, os.ModePerm)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":    err,
			"FilePath": filePath,
			"TaskName": p.itemInfos[0].task.Name(),
		}).Error("Write json represent to file failure.")
		return
	}

	logrus.WithFields(logrus.Fields{
		"FilePath": filePath,
		"TaskName": p.itemInfos[0].task.Name(),
	}).Info("JsonFilePipeline write finish.")
}
