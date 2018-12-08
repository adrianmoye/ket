package main

import (
	"bytes"
	"github.com/Masterminds/sprig"
	"log"
	"text/template"

	"k8s.io/client-go/dynamic"
)

type templateRendererType struct {
	templateName  string
	templateText  string
	templateValue string
	mType         string
	resChan       chan templateRendererType
}

type templateRendererControllerComms struct {
	send chan templateRendererType
	recv chan templateRendererType
}

func (t templateRendererControllerComms) Render(name string, val string) string {

	mesg := templateRendererType{
		templateName: name,
		templateText: val,
		mType:        "templateEval",
		resChan:      make(chan templateRendererType, 10),
	}
	t.send <- mesg
	ret := <-mesg.resChan

	return ret.templateValue
}

func (t templateRendererControllerComms) Destroy() string {
	mesg := templateRendererType{
		mType:   "quit",
		resChan: make(chan templateRendererType, 10),
	}
	t.send <- mesg
	ret := <-mesg.resChan

	return ret.templateValue
}
func renderFunc(fmap template.FuncMap, templateName string, templateText string) string {
	var retVal bytes.Buffer
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Template error [%s]:", err)
		}
	}()

	temp, err := template.New(templateName).Funcs(fmap).Option("missingkey=zero").Parse(templateText)
	err = temp.Execute(&retVal, "")
	if err != nil {
		log.Printf("Err: [%s]\n", err.Error())
	}
	return retVal.String()
}

// this is the main template renderer
func templateRendererHelper(dynamicClient dynamic.Interface, parent templateRendererControllerComms, parentId string, name string) {

	id := get_myID(parentId, "(TR)"+name)
	log.Printf("[%s] New TR.\n", id)

	// func NewTemplateController(dynamicClient dynamic.Interface, parentId string, name string) templateControllerComms {
	tc := NewTemplateController(dynamicClient, id, name)
	q := NewQueryCache(dynamicClient, id)
	f := NewFileMonitor(id)

	templateText := ""
	curentTemplate := ""

	fmap := template.FuncMap{
		"get": func(args ...string) []map[string]interface{} {
			return q.ReadItems(args...)
		},
		"render": func(tName string, tText string) string {
			log.Printf("[%s] Try to eval: [%s]\n", id, tName)
			return tc.Render(tName, tText)
		},
		"writefile": func(filename string, data string) string {
			log.Printf("[%s] Writefile [%s]\n", id, filename)
			f.WriteFile(filename, data)
			log.Printf("[%s] Done Writefile [%s]\n", id, filename)
			return ""
		},
		"readfile": func(filename string) string {
			log.Printf("[%s] Doing Readfile [%s]\n", id, filename)
			return f.ReadFile(filename)
		},
		"exec": func(command ...string) struct {
			stdout   string
			stderr   string
			exitcode int
		} {
			log.Printf("[%s] exec [%s]\n", id, command)
			stdout, stderr, exitcode := Exec(command)
			return struct {
				stdout   string
				stderr   string
				exitcode int
			}{stdout, stderr, exitcode}
		},
		"log": func(str ...string) string {
			log.Printf("[%s]: %s\n", id, str)
			return ""
		},
	}

	// add sprig
	for k, v := range sprig.FuncMap() {
		fmap[k] = v
	}

	renderChecker := func() {

		tempTemplate := renderFunc(fmap, name, templateText)

		if tempTemplate != curentTemplate {

			log.Printf("[%s] Template diff\n", id)

			curentTemplate = tempTemplate

			mesg := templateRendererType{
				templateName: name,
				templateText: templateText,
				mType:        "template update",
			}

			parent.send <- mesg

		}
	}

	for {
		select {
		case recv := <-parent.recv:
			log.Printf("[%s] Message Received from parent[%s].\n", id, recv.templateName)

			switch recv.mType {

			case "templateEval":

				templateText = recv.templateText

				tc.SetUnUsed()
				f.SetUnUsed()

				r := renderFunc(fmap, name, templateText)

				recv.templateValue = r
				recv.resChan <- recv
				curentTemplate = recv.templateValue

				tc.DestroyUnUsed()
				f.DestroyUnUsed()

			case "quit":
				log.Printf("[%s] Quit signal received.\n", id)

				// kill all
				tc.Destroy()
				q.Destroy()
				f.Destroy()

				// notify that we the children are destroyed
				recv.resChan <- recv

				return

			default:
				log.Printf("[%s]Error: Unknown parent message type: [%s]", id, recv)
			}
		case recv := <-tc.recv:
			log.Printf("[%s] Message Received from template child[%s].\n", id, recv)
			switch recv.event {

			case "template update":
				log.Printf("[%s] recevied template update\n", id)

				renderChecker()
			default:
				log.Printf("[%s]Error: Unknown templateController message type: [%s]", id, recv)

				renderChecker()
			}
		case recv := <-q.Event:
			recv = ResourceCacheMessage{}
			log.Printf("[%s] Message Received from query child[%s]\n.", id, recv)

			renderChecker()

		case recv := <-f.recv:
			log.Printf("[%s] Update recevied from file[%s]\n", id, recv.filename)

			renderChecker()
		}
	}
}

func templateRendererCacheConstructor(dynamicClient dynamic.Interface, parentId string, name string, childChan chan templateRendererType) templateRendererControllerComms {
	var child templateRendererControllerComms
	var parent templateRendererControllerComms
	child.send = childChan
	child.recv = make(chan templateRendererType, 100)
	parent.send = child.recv
	parent.recv = child.send

	go templateRendererHelper(dynamicClient, child, parentId, name)

	return parent
}

//
