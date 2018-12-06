package main

import (
	"bytes"
	"fmt"
	"log"
	"text/template"

	"k8s.io/client-go/dynamic"

	"github.com/Masterminds/sprig"
)

type templateCache struct {
	dynamicClient dynamic.Interface
	q             QueryCacheType
	fmap          template.FuncMap
	child         map[string]templateChild
}

type templateCacheMessage struct {
	templateName string
	templateText string
	mType        string
	response     string
	resChan      chan templateCacheMessage
}

type templateChild struct {
	id            string
	name          string
	parentId      string
	dynamicClient dynamic.Interface
	parent        chan templateCacheMessage
	self          chan templateCacheMessage
	unUsed        bool
}

func (t templateChild) templateCacheQuery(templateName string, templateText string) string {
	var resChan chan templateCacheMessage
	resChan = make(chan templateCacheMessage)
	mesg := templateCacheMessage{
		templateName: templateName,
		templateText: templateText,
		mType:        "templateEval",
		response:     "",
		resChan:      resChan,
		//parentChan:   t.parent,
	}
	t.self <- mesg
	t.unUsed = false
	res := <-resChan
	return res.response
}

func (t templateCache) renderFunc(fmap template.FuncMap, templateName string, templateText string) string {
	var retVal bytes.Buffer
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("Template error:", err)
		}
	}()

	temp, err := template.New(templateName).Funcs(fmap).Option("missingkey=zero").Parse(templateText)
	err = temp.Execute(&retVal, "")
	if err != nil {
		log.Printf("Err: [%s]\n", err.Error())
	}
	return retVal.String()
}

func templateHelper(templateComms templateChild) {
	var t templateCache

	id := get_myID(templateComms.parentId, templateComms.name)

	f := NewFileMonitor(id)

	templateComms.id = id
	t.dynamicClient = templateComms.dynamicClient
	q := NewQueryCache(t.dynamicClient, id)
	t.child = make(map[string]templateChild)

	resetKids := func() {
		for kid := range t.child {
			k := t.child[kid]
			k.unUsed = true
			t.child[kid] = k
		}
	}

	killKids := func() {
		for kid := range t.child {
			if t.child[kid].unUsed {
				t.child[kid].self <- templateCacheMessage{mType: "quit"}
				delete(t.child, kid)
			}
		}
	}

	defer func() {
		f.Destroy()
	}()

	t.fmap = template.FuncMap{
		"get": func(args ...string) []map[string]interface{} {
			return q.ReadItems(args...)
		},
		"render": func(tName string, tText string) string {
			if _, ok := t.child[tName]; !ok {
				t.child[tName] = templateCacheConstructor(templateComms, tName)
			}
			log.Printf("[%s] Rendername[%s]\n", id, tName)
			m := t.child[tName].templateCacheQuery(tName, tText)
			tc := t.child[tName]
			tc.unUsed = false
			t.child[tName] = tc
			return m
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
		t.fmap[k] = v
	}

	templateName := ""
	templateText := ""
	curentTemplate := ""

	for {
		select {
		case m := <-f.recv:
			log.Printf("[%s] Update recevied from file[%s]\n", id, m.filename)

			tempTemplate := t.renderFunc(t.fmap, templateName, templateText)
			if tempTemplate != curentTemplate {
				log.Printf("[%s] Template diff\n", id)
				curentTemplate = tempTemplate
				mesg := templateCacheMessage{
					templateName: templateName,
					templateText: templateText,
					mType:        "template update",
					response:     curentTemplate,
				}
				templateComms.parent <- mesg
				log.Printf("[%s] Sent new message from to [%s]\n", id, templateComms.parentId)
			}
		case m := <-templateComms.self:
			switch m.mType {

			case "propogate to child":
				log.Printf("[%s]Recv to child[%s]\n", id, m.response)
				if templateComms.name == m.templateText {
					m.mType = "propogate to parent"
					m.response = fmt.Sprintf("[%s]:Send switch parent:[%s]||", id, m.response)
					templateComms.parent <- m
				} else {
					for _, chld := range t.child {
						m.response = fmt.Sprintf("[%s]:Send to child:[%s]||", id, m.response)
						chld.self <- m
					}
				}

			case "propogate to parent":
				log.Printf("[%s]Recv to parent[%s]\n", id, m.response)
				m.response = fmt.Sprintf("[%s]:Send to parent:[%s]||", id, m.response)
				templateComms.parent <- m

			case "templateEval":
				//templateName = m.templateName
				templateText = m.templateText
				//id = get_myID(parentId, templateName)
				//t.parentMessage = m.parentChan
				resetKids()
				f.SetUnUsed()
				r := t.renderFunc(t.fmap, m.templateName, m.templateText)
				f.DeleteUnUsed()
				killKids()
				m.response = r
				m.resChan <- m
				curentTemplate = r

			case "template update":
				log.Printf("[%s] recevied template update\n", id)

				tempTemplate := t.renderFunc(t.fmap, templateName, templateText)
				if tempTemplate != curentTemplate {
					log.Printf("[%s] Template diff\n", id)
					curentTemplate = tempTemplate
					mesg := templateCacheMessage{
						templateName: templateName,
						templateText: templateText,
						mType:        "template update",
						response:     curentTemplate,
					}
					templateComms.parent <- mesg
					log.Printf("[%s] Sent new message from to [%s]\n", id, templateComms.parentId)
				}
			case "quit":
				log.Printf("[%s] Quit signal received.\n", id)

				// kill templates
				resetKids()
				killKids()
				// destroy query cache
				q.Destroy()
				return
			default:
				log.Printf("[%s]Error: Unknown general message type: [%s]", id, m)

			}
		case m := <-q.Event:
			tR := q.Query[m.query]
			tR.Resource = m.Resource
			q.Query[m.query] = tR
			log.Printf("[%s] Qevent Received [%s]\n", id, m.query)
			tempTemplate := t.renderFunc(t.fmap, templateName, templateText)
			//			log.Printf("Qevent diff [%s][%s][%s]\n", templateName, curentTemplate, tempTemplate)
			if tempTemplate != curentTemplate {
				log.Printf("[%s] Template diff\n", id)
				curentTemplate = tempTemplate
				mesg := templateCacheMessage{
					templateName: templateName,
					templateText: templateText,
					mType:        "template update",
					response:     curentTemplate,
				}
				templateComms.parent <- mesg
				log.Printf("[%s] Sent new message from to [%s]\n", id, templateComms.parentId)

			}
		}
	}
}

func templateCacheConstructor(parent templateChild, name string) templateChild {
	var forChild templateChild
	forChild.dynamicClient = parent.dynamicClient
	forChild.parent = parent.self
	forChild.self = make(chan templateCacheMessage, 50)
	forChild.parentId = parent.id
	forChild.name = name

	go templateHelper(forChild)

	return forChild
}

func eventLoop(dynamicClient dynamic.Interface, templateName string, templateText string) {

	event := make(chan templateCacheMessage, 100)
	id := get_myID("", "main")

	child := templateChild{
		id:            id,
		name:          "",
		self:          event,
		parentId:      "",
		dynamicClient: dynamicClient,
	}

	t := templateCacheConstructor(child, templateName)

	/*
			type templateCacheMessage struct {
		        templateName string
		        templateText string
		        mType        string
		        response     string
		        resChan      chan templateCacheMessage
		        //parentChan   chan templateCacheMessage
		}*/
	/*
		var tf func()
		tf = func() {
			timer1 := time.NewTimer(2 * time.Second)
			<-timer1.C
			ms := templateCacheMessage{
				mType:        "propogate to child",
				templateText: "loadbalancer-config",
				response:     "1",
			}
			t.self <- ms
			go tf()
		}
		go tf()
	*/
	text := t.templateCacheQuery(templateName, templateText)
	log.Printf("new result: [%s]\n", text)
	for {
		select {
		case e := <-event:
			log.Printf("UPDATED Root KubeEventTemplate: [%s]\n", e.response)
			text := t.templateCacheQuery(templateName, templateText)
			log.Printf("new result: [%s]\n", text)
		}

	}
}

//
