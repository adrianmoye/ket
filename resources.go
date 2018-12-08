package main

import (
	"log"
	"reflect"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	// https://stackoverflow.com/questions/47116811/client-go-parse-kubernetes-json-files-to-k8s-structures
	//	discocache "k8s.io/client-go/discovery/cached"
	//"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

type QueryType struct {
	qSchema   schema.GroupVersionResource
	listOpts  metav1.ListOptions
	Namespace string
	id        string
}

type ResourceCacheMessage struct {
	mType    string
	Resource []map[string]interface{}
	query    QueryType
	rFchan   chan ResourceCacheMessage
}

/*
 * Resource Controller
 */

type ResourceControllerComms struct {
	send chan ResourceCacheMessage
	recv chan ResourceCacheMessage
}

func (r ResourceControllerComms) Destroy() string {
	log.Printf("[%s]: Sending destroy [%s].\n", r)
	var mesg ResourceCacheMessage
	mesg.mType = "Destroy"
	mesg.rFchan = make(chan ResourceCacheMessage, 100)
	r.send <- mesg
	response := <-mesg.rFchan
	return response.mType
}

func (r ResourceControllerComms) ReadItems() []map[string]interface{} {
	log.Printf("[%s]: Sending destroy [%s].\n", r)
	var mesg ResourceCacheMessage
	mesg.mType = "Query"
	mesg.rFchan = make(chan ResourceCacheMessage, 100)
	r.send <- mesg
	response := <-mesg.rFchan
	return response.Resource
}

func getName(item map[string]interface{}) string {
	name := ""
	metadata := item["metadata"].(map[string]interface{})
	if _, ok := metadata["namespace"]; ok {
		name += metadata["namespace"].(string) + "/"
	}
	name += metadata["name"].(string)
	return name
}

func ResourceControllerHelper(dynamicClient dynamic.Interface, parent ResourceControllerComms, parentId string, query QueryType) {
	id := get_myID(parentId, "(QC)")
	log.Printf("[%s] New RC: [%s]\n", id, query)

	Resources := make(map[string]map[string]interface{})

	dynamicResource := query.qSchema
	dynamicResourceList, err := dynamicClient.Resource(dynamicResource).Namespace(query.Namespace).List(query.listOpts)
	if err != nil {
		panic(err.Error())
	}

	// lets first populate the cache, then we can watch it
	for _, dR := range dynamicResourceList.Items {
		name := getName(dR.Object)
		Resources[name] = dR.Object //reflect.ValueOf(dR).Field(0).Interface().(map[string]interface{})
	}

	dynamicResourceListChan, err := dynamicClient.Resource(dynamicResource).Namespace(query.Namespace).Watch(query.listOpts)
	if err != nil {
		panic(err.Error())
	}

	watcher := dynamicResourceListChan.ResultChan()
	for {
		select {
		case recv := <-parent.recv:
			switch recv.mType {
			case "Query":

				var keys []string
				for key := range Resources {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				var retval []map[string]interface{}
				for _, key := range keys {
					retval = append(retval, Resources[key])
				}
				recv.Resource = retval

				recv.rFchan <- recv

			case "Destroy":
				log.Printf("[%s] Quit Signal.\n", id)
				dynamicResourceListChan.Stop()
				recv.rFchan <- recv
				return
			default:
			}

		case e := <-watcher:
			//		log.Printf("Qry: [%s]\n", query)
			eventType := reflect.ValueOf(e).Field(0).Interface().(watch.EventType)
			ta := *reflect.ValueOf(e).Field(1).Interface().(*unstructured.Unstructured) //.(*map[string]interface{}) //(*unstructured.Unstructured)
			t := ta.Object
			if (eventType == "ADDED") || (eventType == "MODIFIED") {
				name := getName(t)
				Resources[name] = t
			}
			if eventType == "DELETED" {
				name := getName(t)
				delete(Resources, name)
			}
			log.Printf("[%s] Sending event to parent.\n", id)
			parent.send <- ResourceCacheMessage{query: query}

		}
	}
}

func NewResourceController(dynamicClient dynamic.Interface, parentId string, parentChan chan ResourceCacheMessage, query QueryType) ResourceControllerComms {

	var child ResourceControllerComms
	var parent ResourceControllerComms
	child.send = parentChan
	child.recv = make(chan ResourceCacheMessage, 100)
	parent.send = child.recv
	parent.recv = child.send

	go ResourceControllerHelper(dynamicClient, child, parentId, query)

	return parent
}

/*
 * Query Controller
 */

type QueryControllerComms struct {
	send chan ResourceCacheMessage
	recv chan ResourceCacheMessage
}

func QueryControllerHelper(dynamicClient dynamic.Interface, parent QueryControllerComms, parentId string) {
	id := get_myID(parentId, "(QC)")
	log.Printf("[%s] New QC\n", id)

	child := make(chan ResourceCacheMessage, 100)
	RC := make(map[QueryType]ResourceControllerComms)
	unUsed := make(map[QueryType]bool)

	for {
		select {
		case recv := <-child:
			parent.send <- recv
		case recv := <-parent.recv:
			switch recv.mType {
			case "Query":
				log.Printf("[%s] Received message from parent.\n", id)
				if _, ok := RC[recv.query]; !ok {
					RC[recv.query] = NewResourceController(dynamicClient, id, child, recv.query)
				}
				recv.Resource = RC[recv.query].ReadItems()
				//log.Printf("[%s] TCH[%s]\n", id, ret)
				log.Printf("[%s] sending response to parent.\n", id)
				recv.rFchan <- recv
				unUsed[recv.query] = false

			case "SetUnUsed":
				log.Printf("[%s]: Updating unused.\n", id)
				for item := range unUsed {
					unUsed[item] = true
				}
				recv.rFchan <- recv
			case "DestroyUnUsed":
				for item := range unUsed {
					log.Printf("[%s]: Check [%s].\n", id, item)
					if unUsed[item] {
						log.Printf("[%s]: Sending destroy [%s].\n", id, item)
						RC[item].Destroy()
						delete(RC, item)
						delete(unUsed, item)
					}
				}
				log.Printf("[%s]: Finished Destroy unused.\n", id)
				recv.rFchan <- recv

			case "Destroy":
				log.Printf("[%s]: Received Destroying.\n", id)
				for item := range unUsed {
					log.Printf("[%s]: Check [%s].\n", id, item)
					log.Printf("[%s]: Sending destroy [%s].\n", id, item)
					RC[item].Destroy()
					delete(RC, item)
					delete(unUsed, item)
				}
				log.Printf("[%s]: Finished Destroying.\n", id)
				recv.rFchan <- recv
				return
			default:
			}
		}
	}

}

func (q QueryControllerComms) ReadItems(args ...string) []map[string]interface{} {

	log.Printf("[%s]: Sending ReadItems [%s].\n")
	var query QueryType
	//var res_name string
	//var labels string
	var arg string
	var listOpts metav1.ListOptions
	qSchema := schema.GroupVersionResource{
		Version: "v1",
	}

	rescount := 0

	for len(args) > 0 {
		arg, args = args[0], args[1:]
		switch arg {
		case "-n":
			query.Namespace, args = args[0], args[1:]
		case "-l":
			listOpts.LabelSelector, args = args[0], args[1:]
		default:
			switch rescount {
			case 0:
				qSchema.Resource = arg
			case 1:
				//res_name = arg
			}
			rescount++
		}

	}

	query.qSchema = qSchema
	query.listOpts = listOpts

	var mesg ResourceCacheMessage
	mesg.mType = "Query"
	mesg.query = query
	mesg.rFchan = make(chan ResourceCacheMessage, 100)
	q.send <- mesg
	response := <-mesg.rFchan
	log.Printf("Received query result[%s]\n,")
	return response.Resource
}

func (q QueryControllerComms) SetUnUsed() string {
	log.Printf("[%s]: Sending SetUnUsed [%s].\n", q)
	var mesg ResourceCacheMessage
	mesg.mType = "SetUnUsed"
	mesg.rFchan = make(chan ResourceCacheMessage, 100)
	q.send <- mesg
	response := <-mesg.rFchan
	return response.mType
}

func (q QueryControllerComms) DestroyUnUsed() string {
	log.Printf("[%s]: Sending DestroyUnUsed [%s].\n", q)
	var mesg ResourceCacheMessage
	mesg.mType = "DestroyUnUsed"
	mesg.rFchan = make(chan ResourceCacheMessage, 100)
	q.send <- mesg
	response := <-mesg.rFchan
	return response.mType
}

func (q QueryControllerComms) Destroy() string {
	log.Printf("[%s]: Sending destroy [%s].\n", q)
	var mesg ResourceCacheMessage
	mesg.mType = "Destroy"
	mesg.rFchan = make(chan ResourceCacheMessage, 100)
	q.send <- mesg
	response := <-mesg.rFchan
	return response.mType
}

func NewQueryController(dynamicClient dynamic.Interface, parentId string) QueryControllerComms {
	var child QueryControllerComms
	var parent QueryControllerComms
	child.send = make(chan ResourceCacheMessage, 100)
	child.recv = make(chan ResourceCacheMessage, 100)
	parent.send = child.recv
	parent.recv = child.send

	go QueryControllerHelper(dynamicClient, child, parentId)

	return parent
}

//
