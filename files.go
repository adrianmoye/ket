package main

import (
	"io/ioutil"
	"log"

	"github.com/fsnotify/fsnotify"
)

type fileMonitorType struct {
	filename string
	perms    string
	data     string
	event    string
	status   string
	misc     string
	rFchan   chan fileMonitorType
}

func fileMonitorHelper(f fileMonitorComms, parentId string) {

	id := get_myID(parentId, "(FM)")

	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		log.Printf("[%s]: ERROR[%s]\n", id, err)
	}
	defer watcher.Close()

	unUsed := make(map[string]bool)

	for {
		select {
		case recv := <-f.recv:
			switch recv.event {
			case "write":
				bytearray := []byte(recv.data)
				err := ioutil.WriteFile(recv.filename, bytearray, 0644)
				unUsed[recv.filename] = false
				if err != nil {
					log.Printf("[%s]: ERROR[%s]\n", id, err)
				}
				if err := watcher.Add(recv.filename); err != nil {
					log.Printf("[%s]: ERROR[%s]\n", id, err)
				}
			case "read":
				log.Printf("[%s]: Trying read[%s]\n", id, recv.filename)
				data, err := ioutil.ReadFile(recv.filename)
				unUsed[recv.filename] = false
				if err != nil {
					log.Printf("[%s]: ERROR[%s]\n", id, err)
				}
				recv.data = string(data)
				recv.rFchan <- recv
				if err := watcher.Add(recv.filename); err != nil {
					log.Printf("[%s]: ERROR[%s]\n", id, err)
				}
			case "setUnUsed":
				log.Printf("[%s]: Updating unused\n", id)
				for item := range unUsed {
					unUsed[item] = true
				}
				recv.rFchan <- recv
			case "destroyUnUsed":
				log.Printf("[%s]: Deleting unused\n", id)
				for item := range unUsed {
					if unUsed[item] {
						watcher.Remove(item)
						delete(unUsed, item)
					}
				}
				recv.rFchan <- recv
			case "destroy":
				log.Printf("[%s]: Trying Destroying!\n", id)
				recv.rFchan <- recv
				return
			default:
			}

		// watch for events
		case event := <-watcher.Events:
			log.Printf("[%s]: EVENT! %#v\n", id, event)
			var fm fileMonitorType
			fm.filename = event.Name
			fm.event = "changed"
			f.send <- fm

			// watch for errors
		case err := <-watcher.Errors:
			log.Printf("[%s]: ERROR[%s]\n", id, err)
			var fm fileMonitorType
			//fm.filename = event.Name
			fm.event = "changed"
			f.send <- fm

		}
	}

}

type fileMonitorComms struct {
	send chan fileMonitorType
	recv chan fileMonitorType
}

func (f fileMonitorComms) ReadFile(filename string) string {

	var mesg fileMonitorType
	mesg.filename = filename
	mesg.event = "read"
	mesg.rFchan = make(chan fileMonitorType, 100)

	f.send <- mesg
	response := <-mesg.rFchan

	return response.data
}

func (f fileMonitorComms) WriteFile(filename string, data string) {
	var mesg fileMonitorType
	mesg.filename = filename
	mesg.data = data
	mesg.event = "write"
	f.send <- mesg
}

func (f fileMonitorComms) SetUnUsed() string {
	var mesg fileMonitorType
	mesg.event = "setUnUsed"
	mesg.rFchan = make(chan fileMonitorType, 100)
	f.send <- mesg
	response := <-mesg.rFchan
	return response.event
}

func (f fileMonitorComms) DestroyUnUsed() string {
	var mesg fileMonitorType
	mesg.event = "destroyUnUsed"
	mesg.rFchan = make(chan fileMonitorType, 100)
	f.send <- mesg
	response := <-mesg.rFchan
	return response.event
}

func (f fileMonitorComms) Destroy() string {
	var mesg fileMonitorType
	mesg.event = "destroy"
	mesg.rFchan = make(chan fileMonitorType, 100)
	f.send <- mesg
	response := <-mesg.rFchan
	return response.event
}

func NewFileMonitor(parentId string) fileMonitorComms {
	var child fileMonitorComms
	var parent fileMonitorComms
	child.send = make(chan fileMonitorType, 100)
	child.recv = make(chan fileMonitorType, 100)
	parent.send = child.recv
	parent.recv = child.send

	go fileMonitorHelper(child, parentId)

	return parent
}

//
