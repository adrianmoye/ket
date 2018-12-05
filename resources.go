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
	Resource []map[string]interface{}
	query    QueryType
}
type ResourceCacheType struct {
	id            string
	Resource      []map[string]interface{}
	dynamicClient dynamic.Interface
	query         QueryType
	quit          chan string
}

func NewResourceCache(dynamicClient dynamic.Interface, query QueryType, event chan ResourceCacheMessage, parentId string) ResourceCacheType {
	var r ResourceCacheType
	r.dynamicClient = dynamicClient
	r.query = query
	r.quit = make(chan string)
	Resources := make(map[string]map[string]interface{})

	dynamicResource := query.qSchema
	dynamicResourceList, err := dynamicClient.Resource(dynamicResource).Namespace(r.query.Namespace).List(r.query.listOpts)
	if err != nil {
		panic(err.Error())
	}

	// lets first populate the cache, then we can watch it
	for _, dR := range dynamicResourceList.Items {
		//res := reflect.ValueOf(dR).Field(0).Interface().(unstructured.Unstructured) //(map[string]interface{})
		name := r.getName(dR.Object)
		Resources[name] = dR.Object //reflect.ValueOf(dR).Field(0).Interface().(map[string]interface{})
	}

	// sort the resource map into an ordered array so we can
	// get deterministic outputs in the templates
	GetItems := func(Resources map[string]map[string]interface{}) []map[string]interface{} {
		var keys []string
		for key := range Resources {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var retval []map[string]interface{}
		for _, key := range keys {
			//	v := *reflect.ValueOf(Resources[key]).interface().(*map[string]interface{})
			//	log.Printf("\ntype\n[%s][%s]\n\n\n", key, reflect.ValueOf(Resources[key]).Type())
			//	log.Printf("\nval\n[%s][%s]\n\n\n", key, v)
			retval = append(retval, Resources[key])
		}
		return retval
	}

	r.Resource = GetItems(Resources)

	// listen for events and update cache and propogate them
	go func() {
		id := get_myID(parentId, "RC")
		log.Printf("[%s] Query Watcher Created.\n", id)

		dynamicResourceListChan, err := dynamicClient.Resource(dynamicResource).Namespace(r.query.Namespace).Watch(r.query.listOpts)
		if err != nil {
			panic(err.Error())
		}

		c := dynamicResourceListChan.ResultChan()
		for {
			select {
			case e := <-c:
				//		log.Printf("Qry: [%s]\n", query)
				eventType := reflect.ValueOf(e).Field(0).Interface().(watch.EventType)
				//		log.Printf("New event: [%s]\n", reflect.ValueOf(e).Type())
				ta := *reflect.ValueOf(e).Field(1).Interface().(*unstructured.Unstructured) //.(*map[string]interface{}) //(*unstructured.Unstructured)
				t := ta.Object
				if (eventType == "ADDED") || (eventType == "MODIFIED") {
					//			r.updateItem(t.Object)
					name := r.getName(t)
					Resources[name] = t
				}
				if eventType == "DELETED" {
					//				r.deleteItem(t.Object)
					name := r.getName(t)
					delete(Resources, name)
				}
				r.Resource = GetItems(Resources)
				event <- ResourceCacheMessage{Resource: r.Resource, query: r.query}

			case <-r.quit:
				log.Printf("[%s] Quit Signal.\n", id)
				dynamicResourceListChan.Stop()
				return
			}
		}
	}()
	return r
}
func (r ResourceCacheType) Destroy() {
	close(r.quit)
}
func (r ResourceCacheType) getName(item map[string]interface{}) string {
	name := ""
	metadata := item["metadata"].(map[string]interface{})
	if _, ok := metadata["namespace"]; ok {
		name += metadata["namespace"].(string) + "/"
	}
	name += metadata["name"].(string)
	return name
}

type QueryCacheType struct {
	dynamicClient dynamic.Interface
	parentId      string
	Query         map[QueryType]ResourceCacheType
	Event         chan ResourceCacheMessage
}

//func (q QueryCacheType) ReadItems(query QueryType) []map[string]interface{} {
func (q QueryCacheType) ReadItems(args ...string) []map[string]interface{} {
	// return q.ReadItems(QueryType{qSchema: schema.GroupVersionResource{Version: "v1", Resource: resource}}

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

	if _, ok := q.Query[query]; !ok {
		q.Query[query] = NewResourceCache(q.dynamicClient, query, q.Event, q.parentId)
	}
	return q.Query[query].Resource
}
func (q QueryCacheType) DeleteQuery(query QueryType) {
	q.Query[query].Destroy()
	delete(q.Query, query)
}
func (q QueryCacheType) Destroy() {
	for query := range q.Query {
		q.DeleteQuery(query)
	}
}
func NewQueryCache(dynamicClient dynamic.Interface, parentId string) QueryCacheType {
	var q QueryCacheType
	q.Query = make(map[QueryType]ResourceCacheType)
	q.dynamicClient = dynamicClient
	q.Event = make(chan ResourceCacheMessage)
	q.parentId = parentId
	return q
}

//
