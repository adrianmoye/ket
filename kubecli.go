package main

import (
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	//"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type kubeCli struct {
	config *rest.Config
	dc     dynamic.Interface
}

func (c kubeCli) getConfig() *rest.Config {
	return c.config
}
func (c kubeCli) getDc() dynamic.Interface {
	return c.dc
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

	//  Create a Dynamic Client to interface with CRDs.
	dynamicClient, err := dynamic.NewForConfig(config)

	c.dc = dynamicClient

	return c
}

//
