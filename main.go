package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	//"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func getGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

func get_myID(parent string, self string) string {
	return fmt.Sprintf("%s.%s[%d]", parent, self, getGID())
}

func main() {

	var kubeconfig *string

	namespace := flag.String("namespace", "kube-system", "Namespace of the configmap to use.")
	name := flag.String("name", "inittemplate", "The initial config map name to use.")

	goTemplate := flag.String("go-template", "", "Override the default template.")

	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		// hopefully this'll give us an incluster config if that doesn't work
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	//  Create a Dynamic Client to interface with CRDs.
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	const initTemplate = `
{{- $v := get "-n" "%s" "configmaps" -}}
{{- range $v -}}
{{- if eq .metadata.name "%s" -}}
{{- render .metadata.name .data.template -}}
{{- end -}}
{{- end -}}
`

	templateName := "initTemplate"
	templateText := ""
	t := ""
	t = *goTemplate

	if t == "" {
		templateText = fmt.Sprintf(initTemplate, *namespace, *name)
	} else {
		templateText = *goTemplate
	}
	eventLoop(dynamicClient, templateName, templateText)
}

//
