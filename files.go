package main

import (
	"fmt"
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

func (f fileMonitorComms) readfile(filename string) string {

	var mesg fileMonitorType
	mesg.filename = filename
	mesg.event = "read"
	mesg.rFchan = make(chan fileMonitorType, 100)

	f.send <- mesg
	response := <-mesg.rFchan

	return response.data
}

func (f fileMonitorComms) writefile(filename string, data string) {
	var mesg fileMonitorType
	mesg.filename = filename
	mesg.data = data
	mesg.event = "write"
	f.send <- mesg
}

func fileMonitorHelper(f fileMonitorComms) {
	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		fmt.Println("ERROR", err)
	}
	defer watcher.Close()

	filename := ""
	//	watching := false

	for {
		select {
		case recv := <-f.recv:
			if recv.event == "write" {
				bytearray := []byte(recv.data)
				err := ioutil.WriteFile(recv.filename, bytearray, 0644)
				if err != nil {
					fmt.Println("ERROR", err)
				}
				if err := watcher.Add(recv.filename); err != nil {
					fmt.Println("ERROR", err)
				}
			}
			if recv.event == "read" {
				log.Printf("trying read[%s]\n", recv.filename)
				data, err := ioutil.ReadFile(recv.filename)
				if err != nil {
					fmt.Println("ERROR", err)
				}
				recv.data = string(data)
				//log.Printf("data is[%s]\n", recv.data)
				recv.rFchan <- recv
				if err := watcher.Add(recv.filename); err != nil {
					fmt.Println("ERROR", err)
				}
				filename = recv.filename
				/*
					if filename != recv.filename && !watching {
						watching = true
						log.Printf("add notify[%s]\n", recv.filename)
						filename = recv.filename
						if err := watcher.Add(recv.filename); err != nil {
							fmt.Println("ERROR", err)
						}
					}*/
			}

		// watch for events
		case event := <-watcher.Events:
			log.Printf("EVENT! %#v\n", event)
			var fm fileMonitorType
			fm.filename = filename
			fm.event = "changed"
			f.send <- fm

			// watch for errors
		case err := <-watcher.Errors:
			fmt.Println("ERROR", err)
			var fm fileMonitorType
			fm.filename = filename
			fm.event = "changed"
			f.send <- fm

		}
	}

}

type fileMonitorComms struct {
	send chan fileMonitorType
	recv chan fileMonitorType
}

func NewFileMonitor(parentChan chan fileMonitorType) fileMonitorComms {
	var child fileMonitorComms
	var parent fileMonitorComms
	child.send = parentChan
	child.recv = make(chan fileMonitorType, 100)
	parent.send = child.recv
	parent.recv = child.send

	go fileMonitorHelper(child)

	return parent
}

//
