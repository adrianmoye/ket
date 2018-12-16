package main

import (
	"encoding/json"
	"log"

	// https://stackoverflow.com/questions/47116811/client-go-parse-kubernetes-json-files-to-k8s-structures
	//"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type kubeCli struct {
	config *rest.Config
	dc     dynamic.Interface
	disco  discovery.DiscoveryInterface
	ag     []*restmapper.APIGroupResources
	cs     *kubernetes.Clientset
	rm     meta.RESTMapper
}

/*
func (c kubeCli) getConfig() *rest.Config {
	return c.config
}
func (c kubeCli) getDc() dynamic.Interface {
	return c.dc
}
*/
func (c kubeCli) jsonToObjects(def string) (*meta.RESTMapping, map[string]interface{}, error) {

	jsonV, err := yaml.ToJSON([]byte(def))
	if err != nil {
		log.Printf("ERROR: invalid yaml: [%s]\n", err)
	}
	log.Println("raw: ", string([]byte(jsonV)))
	versions := &runtime.VersionedObjects{}
	obj, gvk, err := unstructured.UnstructuredJSONScheme.Decode([]byte(jsonV), nil, versions)
	log.Println("obj: ", obj)
	if err != nil {
		log.Printf("obj err: [%s]\n", err)
	}

	mapping, err := c.rm.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		log.Printf("restmappererrordisco: [%s]\n", err)
	}

	var blob interface{}
	if err := json.Unmarshal(jsonV, &blob); err != nil {
		log.Printf("jsonunmarshal: [%s]\n", err)
	}

	return mapping, blob.(map[string]interface{}), nil
}

func (c kubeCli) createOrUpdate(opp string, def string) map[string]interface{} {

	var unstruct unstructured.Unstructured

	mapping, usObj, err := c.jsonToObjects(def)

	unstruct.Object = usObj
	log.Println("unstruct:", unstruct)

	apiresourcelist, err := c.disco.ServerResources()
	if err != nil {
		log.Printf("apireslisterr: [%s]\n", err)
	}
	var myapiresource metav1.APIResource
	for _, apiresourcegroup := range apiresourcelist {
		if apiresourcegroup.GroupVersion == mapping.GroupVersionKind.Version {
			for _, apiresource := range apiresourcegroup.APIResources {
				// log.Printf("found apiresource: [%s]\n", apiresource)

				if apiresource.Name == mapping.Resource.Resource && apiresource.Kind == mapping.GroupVersionKind.Kind {
					myapiresource = apiresource
					log.Printf("recording found apiresource: [%s]\n", apiresource)
				}
			}
		}
	}
	log.Printf("myapiresource: [%s]\n", myapiresource)

	var gvr schema.GroupVersionResource
	gvr.Version = myapiresource.Version

	ns := ""
	if md, ok := unstruct.Object["metadata"]; ok {
		metadata := md.(map[string]interface{})
		if internalns, ok := metadata["namespace"]; ok {
			ns = internalns.(string)
		}
	}
	if vers, ok := unstruct.Object["apiVersion"]; ok {
		gvr.Version = vers.(string)
	}

	gvr.Group = myapiresource.Group
	gvr.Resource = myapiresource.Name

	restconfig := c.config
	//	restconfig.GroupVersion = &schema.GroupVersion{
	//		Group:   mapping.GroupVersionKind.Group,
	//		Version: mapping.GroupVersionKind.Version,
	//	}
	dclient, err := dynamic.NewForConfig(restconfig)
	if err != nil {
		log.Printf("dynamicclientfromconfig: [%s]\n", err)
	}

	log.Printf("mygvr: [%s]\n", gvr)
	res := dclient.Resource(gvr)
	log.Println(res)
	var us *unstructured.Unstructured
	if opp == "create" {
		us, err = res.Namespace(ns).Create(&unstruct, metav1.CreateOptions{})
	} else {
		us, err = res.Namespace(ns).Update(&unstruct, metav1.UpdateOptions{})
	}
	if err != nil {
		log.Printf("------------------------------\n")
		log.Printf("try to error: [%s][%s]\n", opp, err)
		log.Printf("gvr: [%s]\n", gvr)
		log.Printf("unstruct [%s]\n", unstruct)
		log.Printf("------------------------------\n")
		return nil
	}
	log.Println("unstruct response:", us)
	return us.Object
}

func (c kubeCli) Create(def string) map[string]interface{} {
	return c.createOrUpdate("create", def)
}

func (c kubeCli) Update(def string) map[string]interface{} {
	return c.createOrUpdate("update", def)
}

func (c kubeCli) UpdateStatus(def string) map[string]interface{} {
	var unstruct unstructured.Unstructured
	unstruct.Object = make(map[string]interface{})
	return unstruct.Object
}

func (c kubeCli) Patch(def string) map[string]interface{} {
	var unstruct unstructured.Unstructured
	unstruct.Object = make(map[string]interface{})
	return unstruct.Object
}

func (c kubeCli) Delete(args ...string) string {
	var arg string
	var listOpts metav1.ListOptions
	qSchema := schema.GroupVersionResource{
		Version: "v1",
	}

	res_name := ""
	namespace := ""
	rescount := 0

	for len(args) > 0 {
		arg, args = args[0], args[1:]
		switch arg {
		case "-n":
			namespace, args = args[0], args[1:]
		case "-l":
			listOpts.LabelSelector, args = args[0], args[1:]
		default:
			switch rescount {
			case 0:
				qSchema.Resource = arg
			case 1:
				res_name = arg
			}
			rescount++
		}

	}
	var deleteOptions metav1.DeleteOptions
	//deleteOptions = make(metav1.DeleteOptions)
	err := c.dc.Resource(qSchema).Namespace(namespace).Delete(res_name, &deleteOptions)

	if err != nil {
		log.Printf("ERROR DELETE ns[%s] resname[%s] qschema[%s]\n", namespace, res_name, qSchema)
		log.Printf("ERROR, can't delete resource: [%s]\n", err.Error())
	}

	return ""
}

func (c kubeCli) List(query QueryType) *unstructured.UnstructuredList {

	dynamicResourceList, err := c.dc.Resource(query.qSchema).Namespace(query.Namespace).List(query.listOpts)
	if err != nil {
		log.Printf("Error, can't list resource: [%s]\n", err.Error())
	}

	return dynamicResourceList
}

func (c kubeCli) Watch(query QueryType) watch.Interface {

	dynamicResourceListChan, err := c.dc.Resource(query.qSchema).Namespace(query.Namespace).Watch(query.listOpts)
	if err != nil {
		panic(err.Error())
		log.Printf("Error, can't watch resource: [%s]\n", err.Error())
	}

	return dynamicResourceListChan
}

func NewApiCon(kubeconfig *string) kubeCli {
	var c kubeCli

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		// hopefully this'll give us an incluster config if that doesn't work
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}
	c.config = config

	clientSet, err := kubernetes.NewForConfig(c.config)
	if err != nil {
		panic(err.Error())
	}
	c.cs = clientSet

	//  Create a Dynamic Client to interface with CRDs.
	dynamicClient, err := dynamic.NewForConfig(c.config)
	if err != nil {
		panic(err.Error())
	}
	c.dc = dynamicClient

	disco := c.cs.Discovery()
	c.disco = disco

	apigroups, err := restmapper.GetAPIGroupResources(c.disco)
	if err != nil {
		log.Printf("[%s]\n", err)
	}
	c.ag = apigroups

	rm := restmapper.NewDiscoveryRESTMapper(c.ag)
	c.rm = rm
	/*
	   	const testConfigmap = `
	   apiVersion: v1
	   kind: ConfigMap
	   metadata:
	     name: testconfigmap
	     namespace: default
	   data:
	     some: "data goes here."
	   `
	   	_ = c.Create(testConfigmap)

	   	const testConfigmap2 = `
	   apiVersion: v1
	   kind: ConfigMap
	   metadata:
	     name: testconfigmap
	     namespace: default
	   data:
	     some: "updated data goes here."
	   `
	   	_ = c.Update(testConfigmap2)

	   	_ = c.Delete("-n", "default", "configmaps", "testconfigmap")

	*/

	return c
}

//
