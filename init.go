package main

import (
	"log"
)

func templateInit(kc kubeCli, templateName string, templateText string) {

	id := get_myID("", "main")

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

	tc := NewTemplateController(kc, id, "main")

	resp := tc.Render(templateName, templateText)

	log.Printf("new result: [%s]\n", resp)
	for {
		select {
		case recv := <-tc.recv:
			log.Printf("UPDATED Root KubeEventTemplate: [%s]\n", recv)
			resp := tc.Render(templateName, templateText)
			log.Printf("new result: [%s]\n", resp)
		}

	}
}

//
