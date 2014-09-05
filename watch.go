package transloadit

import (
	"errors"
	"fmt"
	"gopkg.in/fsnotify.v1"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"
)

type WatchOptions struct {
	Input      string
	Output     string
	Watch      bool
	TemplateId string
	NotifyUrl  string
	Steps      map[string]map[string]interface{}
}

type Watcher struct {
	client     *Client
	options    *WatchOptions
	stopped    bool
	Error      chan error
	Done       chan *AssemblyInfo
	end        chan bool
	lastEvents map[string]time.Time
}

func (client *Client) Watch(options *WatchOptions) *Watcher {

	watcher := &Watcher{
		client:     client,
		options:    options,
		Error:      make(chan error),
		Done:       make(chan *AssemblyInfo),
		end:        make(chan bool),
		lastEvents: make(map[string]time.Time),
	}

	watcher.start()

	return watcher

}

func (watcher *Watcher) start() {

	watcher.processDir()

	if watcher.options.Watch {
		go watcher.startWatcher()
	}

}

func (watcher *Watcher) Stop() {

	if watcher.stopped {
		return
	}

	watcher.stopped = true

	watcher.end <- true
	close(watcher.Done)
	close(watcher.Error)
	close(watcher.end)
}

func (watcher *Watcher) processDir() {

	files, err := ioutil.ReadDir(watcher.options.Input)
	if err != nil {
		watcher.error(err)
		return
	}

	input := watcher.options.Input

	for _, file := range files {
		if !file.IsDir() {
			go watcher.processFile(path.Join(input, file.Name()))
		}
	}

}

func (watcher *Watcher) processFile(name string) {

	file, err := os.Open(name)
	if err != nil {
		watcher.error(err)
		return
	}

	assembly := watcher.client.CreateAssembly()

	if watcher.options.TemplateId != "" {
		assembly.TemplateId = watcher.options.TemplateId
	}

	if watcher.options.NotifyUrl != "" {
		assembly.NotifyUrl = watcher.options.NotifyUrl
	}

	for name, step := range watcher.options.Steps {
		assembly.AddStep(name, step)
	}

	assembly.Blocking = true

	assembly.AddReader("file", path.Base(name), file)

	info, err := assembly.Upload()
	if err != nil {
		watcher.error(err)
		return
	}

	if info.Error != "" {
		watcher.error(errors.New(info.Error))
		return
	}

	for stepName, results := range info.Results {
		for index, result := range results {
			go func() {
				watcher.downloadResult(stepName, index, result)
				watcher.Done <- info
			}()
		}
	}
}

func (watcher *Watcher) downloadResult(stepName string, index int, result *FileInfo) {

	fileName := fmt.Sprintf("%s_%d_%s", stepName, index, result.Name)

	resp, err := http.Get(result.Url)
	if err != nil {
		watcher.error(err)
		return
	}

	defer resp.Body.Close()

	out, err := os.Create(path.Join(watcher.options.Output, fileName))
	if err != nil {
		watcher.error(err)
		return
	}

	defer out.Close()

	io.Copy(out, resp.Body)

}

func (watcher *Watcher) startWatcher() {

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		watcher.error(err)
	}

	defer fsWatcher.Close()

	if err = fsWatcher.Add(watcher.options.Input); err != nil {
		watcher.error(err)
	}

	go func() {
		for {

			if watcher.stopped {
				return
			}

			time.Sleep(time.Second)
			now := time.Now()

			for name, lastEvent := range watcher.lastEvents {
				diff := now.Sub(lastEvent)
				if diff > (time.Millisecond * 500) {
					watcher.processFile(name)
				}
			}

		}
	}()

	for {
		select {
		case <-watcher.end:
			return
		case err := <-fsWatcher.Errors:
			watcher.error(err)
		case evt := <-fsWatcher.Events:
			watcher.lastEvents[evt.Name] = time.Now()
		}
	}

}

func (watcher *Watcher) error(err error) {
	watcher.Error <- err
}