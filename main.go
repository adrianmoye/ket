package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

	kc := NewApiCon(kubeconfig)

	const initTemplate = `{{- render "init-template" (index (get "-n" "%s" "configmaps" "%s") 0).data.template -}}`

	templateName := "initTemplate"
	templateText := ""

	if goTemplate == nil {
		templateText = fmt.Sprintf(initTemplate, *namespace, *name)
	} else {
		templateText = *goTemplate
	}
	templateInit(kc, templateName, templateText)
}

//
