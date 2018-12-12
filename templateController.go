package main

import (
	"log"
)

/*
 * The template controller section, this doesn't actually do any template stuff.
 * Its only purpose is to manage the complexities of the comms and children.
 */

type templateControllerType struct {
	name   string
	value  string
	event  string
	rFchan chan templateControllerType
}

type templateControllerComms struct {
	send chan templateControllerType
	recv chan templateControllerType
}

func templateControllerHelper(kc kubeCli, parent templateControllerComms, parentId string, name string) {

	id := get_myID(parentId, "(TC)")
	log.Printf("[%s] New TC\n", id)

	childMessage := make(chan templateRendererType, 100)
	templates := make(map[string]templateRendererControllerComms)
	unUsed := make(map[string]bool)

	for {
		select {
		case recv := <-childMessage:
			// yes, zero this is just to notify and shit.
			recv = templateRendererType{}
			log.Printf("[%s] Received message from child [%s].\n", id, recv)
			var mesg templateControllerType
			parent.send <- mesg
		case recv := <-parent.recv:
			switch recv.event {
			case "render":
				log.Printf("[%s] Received message from parent.\n", id)
				if _, ok := templates[recv.name]; !ok {
					templates[recv.name] = templateRendererCacheConstructor(kc, id, recv.name, childMessage)
				}
				var ret templateControllerType
				ret.value = templates[recv.name].Render(recv.name, recv.value)
				//log.Printf("[%s] TCH[%s]\n", id, ret)
				recv.rFchan <- ret
				unUsed[recv.name] = false
			case "setUnUsed":
				log.Printf("[%s]: Updating unused.\n", id)
				for item := range unUsed {
					unUsed[item] = true
				}
				recv.rFchan <- recv
			case "destroyUnUsed":
				log.Printf("[%s]: (1643)Receive destroy unused.\n", id)
				for item := range unUsed {
					log.Printf("[%s]: Check [%s].\n", id, item)
					if unUsed[item] {
						log.Printf("[%s]: Sending destroy [%s].\n", id, item)
						templates[item].Destroy()
						delete(templates, item)
						delete(unUsed, item)
					}
				}
				log.Printf("[%s]: Finished Destroy unused.\n", id)
				recv.rFchan <- recv
			case "destroy":
				log.Printf("[%s]: Destroying Children.\n", id)
				for item := range unUsed {
					templates[item].Destroy()
					delete(templates, item)
					delete(unUsed, item)
				}
				log.Printf("[%s]: Finished destroying Children.\n", id)
				recv.rFchan <- recv
				return
			default:
				log.Printf("[%s]: Unknown Message [%s].\n", id, recv)
			}

		}
	}

}

func (t templateControllerComms) Render(name string, value string) string {

	var mesg templateControllerType
	mesg.name = name
	mesg.value = value
	mesg.event = "render"
	mesg.rFchan = make(chan templateControllerType, 100)

	t.send <- mesg
	response := <-mesg.rFchan

	return response.value
}

func (t templateControllerComms) SetUnUsed() string {
	var mesg templateControllerType
	mesg.event = "setUnUsed"
	mesg.rFchan = make(chan templateControllerType, 100)
	t.send <- mesg
	response := <-mesg.rFchan
	return response.event
}

func (t templateControllerComms) DestroyUnUsed() string {
	var mesg templateControllerType
	log.Printf("[%s]: Sending destroy unused[%s].\n", t)
	mesg.event = "destroyUnUsed"
	mesg.rFchan = make(chan templateControllerType, 100)
	t.send <- mesg
	response := <-mesg.rFchan
	return response.event
}

func (t templateControllerComms) Destroy() string {
	/*
	   type templateControllerType struct {
	           name   string
	           value  string
	           event  string
	           rFchan chan templateControllerType
	   }
	*/

	log.Printf("[%s]: Sending destroy [%s].\n", t)
	var mesg templateControllerType
	mesg.event = "destroy"
	mesg.rFchan = make(chan templateControllerType, 100)
	t.send <- mesg
	response := <-mesg.rFchan
	return response.event
}

func NewTemplateController(kc kubeCli, parentId string, name string) templateControllerComms {
	var child templateControllerComms
	var parent templateControllerComms
	child.send = make(chan templateControllerType, 100)
	child.recv = make(chan templateControllerType, 100)
	parent.send = child.recv
	parent.recv = child.send

	go templateControllerHelper(kc, child, parentId, name)

	return parent
}

//
