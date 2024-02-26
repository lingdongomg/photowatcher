package filewatcher

import (
	"github.com/fsnotify/fsnotify"
	"log"
	"os"
	"path/filepath"
	"time"
)

type MultiWatcher struct {
	watcher   *fsnotify.Watcher
	paths     map[string]bool
	snapshots map[string]*FileSnapshot
}

func NewMultiWatcher() (*MultiWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	multiWatcher := &MultiWatcher{
		watcher:   watcher,
		paths:     make(map[string]bool),
		snapshots: make(map[string]*FileSnapshot),
	}
	go fileChangeHandler(multiWatcher)
	return multiWatcher, nil
}

func fileChangeHandler(multiWatcher *MultiWatcher) {
	pending := false
	eventCh := make(chan bool, 1)
	var timer *time.Timer
	for {
		select {
		case _, ok := <-multiWatcher.watcher.Events:
			if !ok {
				log.Println("Close the listening event pipeline")
				return
			}

			if !pending {
				pending = true
				timer = time.AfterFunc(5*time.Second, func() {
					eventCh <- true
				})
			} else {
				timer.Stop()
				timer = time.AfterFunc(5*time.Second, func() {
					eventCh <- true
				})
			}

		case err, ok := <-multiWatcher.watcher.Errors:
			if !ok {
				log.Println("Close the pipeline listening for errors")
				return
			}
			log.Println("Listen for error pipeline discovery:", err)

		case <-eventCh:
			pending = false
			log.Println("File change event handling")
			for path := range multiWatcher.paths {
				snapshot := multiWatcher.snapshots[path]
				newSnapshot := NewFileSnapshot(path)
				diffs := snapshot.Diff(newSnapshot)
				if len(diffs) > 0 {
					multiWatcher.snapshots[path] = newSnapshot
					for _, diff := range diffs {
						log.Println("File Change:", diff)
						if cb != nil {
							cbe := CallBackEvent{Path: diff.AbsPath, Op: diff.Op}
							cb.OnPathChanged(cbe)
						} else {
							log.Println("No callback function")
						}
						// 如果diff.AbsPath是文件夹，将其添加到watcher的监听路径
						fileInfo, err := os.Stat(diff.AbsPath)
						if err != nil {
							continue
						}
						if fileInfo.IsDir() {
							err := multiWatcher.watcher.Add(diff.AbsPath)
							if err != nil {
								log.Println("Failed to add directory to listening path:", err)
							} else {
								log.Println("Add a listening path:", diff.AbsPath)
							}
						}
					}
				}
			}
		}
	}
}

func (mw *MultiWatcher) Add(path string) error {
	if _, ok := mw.paths[path]; ok {
		return nil
	}
	err := mw.watcher.Add(path)
	if err != nil {
		return err
	}
	mw.paths[path] = true
	mw.snapshots[path] = NewFileSnapshot(path)
	traverseDir(mw.watcher, path)
	return nil
}

func traverseDir(watcher *fsnotify.Watcher, path string) {
	files, err := os.ReadDir(path)
	if err != nil {
		return
	}
	for _, file := range files {
		fp := filepath.Join(path, file.Name())
		if file.IsDir() {
			err := watcher.Add(fp)
			if err != nil {
				return
			}
			log.Println("Adding a listening path:", fp)
			traverseDir(watcher, fp)
		}
	}
}

func (mw *MultiWatcher) Remove(path string) error {
	if _, ok := mw.paths[path]; !ok {
		return nil
	}
	err := mw.watcher.Remove(path)
	if err != nil {
		return err
	}
	delete(mw.paths, path)
	delete(mw.snapshots, path)
	return nil
}

func (mw *MultiWatcher) Close() error {
	for path := range mw.paths {
		err := mw.Remove(path)
		if err != nil {
			log.Println("error removing", path, ":", err)
		}
	}
	log.Println("Close one watcher")
	return mw.watcher.Close()
}