package main

import (
	"encoding/json"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	cs     *kubernetes.Clientset
}

func (c kubeCli) jsonToObjects(def string) (map[string]interface{}, error) {

	jsonV, err := yaml.ToJSON([]byte(def))
	if err != nil {
		log.Printf("ERROR: invalid yaml: [%s]\n", err)
	}
	log.Println("raw: ", string([]byte(jsonV)))

	var blob interface{}
	if err := json.Unmarshal(jsonV, &blob); err != nil {
		log.Printf("jsonunmarshal: [%s]\n", err)
	}

	return blob.(map[string]interface{}), nil
}

func (c kubeCli) getGVR(search string) schema.GroupVersionResource {
	var gvr schema.GroupVersionResource

	inSlice := func(search string, slice []string) bool {
		for _, i := range slice {
			if search == i {
				return true
			}
		}
		return false
	}

	agrList, err := restmapper.GetAPIGroupResources(c.disco)
	if err != nil {
		log.Printf("[%s]\n", err)
	}

	log.Printf("search GVR: [%s]\n", search)
	for _, agr := range agrList {
		for _, vr := range agr.VersionedResources {
			for _, vri := range vr {
				if (vri.Name == search) || (vri.SingularName == search) || (vri.Kind == search) || (inSlice(search, vri.ShortNames)) {
					log.Printf("Found GVR: [%s]\n", vri)
					gvr.Resource = vri.Name
					gvr.Group = agr.Group.Name
					gvr.Version = agr.Group.PreferredVersion.Version
					return gvr
				}
			}
		}
	}
	log.Printf("not found GVR: [%s]\n", search)

	return gvr
}

func (c kubeCli) createOrUpdate(opp string, def string) map[string]interface{} {

	var unstruct unstructured.Unstructured

	usObj, err := c.jsonToObjects(def)

	unstruct.Object = usObj
	log.Println("unstruct:", unstruct)

	var gvr schema.GroupVersionResource
	ns := ""
	if kind, ok := unstruct.Object["kind"]; ok {
		gvr = c.getGVR(kind.(string))
	}
	if md, ok := unstruct.Object["metadata"]; ok {
		metadata := md.(map[string]interface{})
		if internalns, ok := metadata["namespace"]; ok {
			ns = internalns.(string)
		}
	}
	if vers, ok := unstruct.Object["apiVersion"]; ok {
		gvr.Version = vers.(string)
	}

	log.Printf("mygvr: [%s]\n", gvr)
	var us *unstructured.Unstructured
	if opp == "create" {
		us, err = c.dc.Resource(gvr).Namespace(ns).Create(&unstruct, metav1.CreateOptions{})
	} else {
		us, err = c.dc.Resource(gvr).Namespace(ns).Update(&unstruct, metav1.UpdateOptions{})
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
				//	qSchema.Resource = arg
				qSchema = c.getGVR(arg)
			case 1:
				res_name = arg
			}
			rescount++
		}

	}

	var deleteOptions metav1.DeleteOptions
	err := c.dc.Resource(qSchema).Namespace(namespace).Delete(res_name, &deleteOptions)

	if err != nil {
		log.Printf("ERROR DELETE ns[%s] resname[%s] qschema[%s]\n", namespace, res_name, qSchema)
		log.Printf("ERROR, can't delete resource: [%s]\n", err.Error())
	}

	return ""
}

func (c kubeCli) List(query QueryType) *unstructured.UnstructuredList {

	dynamicResourceList, err := c.dc.Resource(c.getGVR(query.qSchema.Resource)).Namespace(query.Namespace).List(query.listOpts)
	if err != nil {
		log.Printf("Error, can't list resource: [%s]\n", err.Error())
	}

	return dynamicResourceList
}

func (c kubeCli) Watch(query QueryType) watch.Interface {

	dynamicResourceListChan, err := c.dc.Resource(c.getGVR(query.qSchema.Resource)).Namespace(query.Namespace).Watch(query.listOpts)
	if err != nil {
		panic(err.Error())
		log.Printf("Error, can't watch resource: [%s]\n", err.Error())
	}

	return dynamicResourceListChan
}

func (c kubeCli) testCM() {
	const testConfigmap = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: testconfigmap
  namespace: default
data:
  some: "updated data goes here."
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

	_ = c.Delete("-n", "default", "cm", "testconfigmap")
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

	return c
}

//
