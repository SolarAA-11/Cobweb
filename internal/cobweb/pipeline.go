package cobweb

import (
	"encoding/json"
	"fmt"
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
	fmt.Println(item.data)
}

func (p *StdoutPipeline) Close() {

}

const filePipelineDefaultFolderName = "filepipeline"

type JsonFilePipeline struct {
	locker sync.Mutex
	items  []*Item
}

func (p *JsonFilePipeline) Pipe(item *Item) {
	p.locker.Lock()
	defer p.locker.Unlock()
	p.items = append(p.items, item)
}

func (p *JsonFilePipeline) Close() {
	if len(p.items) == 0 {
		return
	}

	fileName := fmt.Sprintf("%v-%v.json", p.items[0].task.Name(), time.Now().Format("2006-01-02 15-04-05"))
	filePath := path.Join("instance", "cobweb-pipe", "json-file-pipeline", fileName)
	err := os.MkdirAll(path.Dir(filePath), os.ModeDir)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":    err,
			"FilePath": filePath,
			"TaskName": p.items[0].task.Name(),
		}).Error("JsonFilePipeline create directory failure.")
		return
	}

	f, err := os.Create(filePath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":    err,
			"FilePath": filePath,
			"TaskName": p.items[0].task.Name(),
		}).Error("JsonFilePipeline create file failure.")
		return
	}
	defer f.Close()

	dataArray := make([]map[string]string, len(p.items))
	for _, item := range p.items {
		dataArray = append(dataArray, item.data)
	}

	jStr, err := json.Marshal(dataArray)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Error":    err,
			"FilePath": filePath,
			"TaskName": p.items[0].task.Name(),
		}).Error("Json marshal item data array failure.")
		return
	}

	_, err = f.Write(jStr)
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
