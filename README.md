Kubernetes Event Template
=========================

It's a simple tool to watch a K8s cluster state and render go-templates
against it. The templates and be used to manipulate whatever you wish.

**WARNING** Don't use this! It's my first go project, and I haven't
done any typed language programming in a **LONG** time, so I thought
it'd be fun as a starter with much interface{} and go routines!
The code is **LOL**worthy, so please help educate me if you're
looking at it!

To use it:

    ./build.sh
    cat examples/*.yaml | kubectl apply -f -
    ./ket-darwin-amd64 --namespace=default --name=init-template
	

This will read the content of a template "template" value of the 
"init-template" configmap in the default namespace. It will then
evaluate it.

Use the default go-template language to do what you wish, we
provide the following primitives to allow you to do this:

* `<value> := get ["-n" <namespace>] <resource type>`: This provides
an ordered list of all of the matching resources. Optionally
provide `"-n" $namespace`.
* `<value> := create <yaml>`: Creates a resource according to a yaml
definition. Returns nil on failure. Returns the object on success.
* `<value> := update <yaml>`: Updates a resource according to a yaml
definition. Returns nil on failure. Returns the object on success.
* `delete ["-n" <namespace>] <resource type> <resource name>`: Deletes
desired resource.
* `render <name> <template text>`: This will render a sub
template, with some value you've acquired with other queries,
returning a string.
* `writefile <filename> <content>`: This lets you write a file,
if the file changes, it'll automatically notify the template,
it can then re-render it if it sees a problem, not it won't
actually re-update the file, you need to read it and decide
what to do.
* `<value> := readfile <filename>`: This reads a file and
returns the content, it will automatically notify the template
if this changes.
* `<stdout, stderr, exitcode> := exec <cmd> <... args>`: executes
a command, and returns the values as those names. Times out after
30 seconds.
* `log <...params>`: logs stuff from the current template.
* Additional sprig functions: https://github.com/Masterminds/sprig

When using a resource you're watching changes, it will automatically
tell it's parent that it's changed, the parent will then re-render
itself, if that value has changed it will notify its parent.
This will keep going down until it goes to the root resource.

This is the bootstrap template for everything:

	    const initTemplate = `
	{{- $v := get "-n" "%s" "configmaps" -}}
	{{- range $v -}}
	{{- if eq .metadata.name "%s" -}}
	{{- render .metadata.name .data.template -}}
	{{- end -}}
	{{- end -}}
	`

or another example:

	./ket-darwin-amd64 --go-template='{{- $v := get "-n" "default" "configmaps" -}}
	{{- range $v -}}
	{{- if eq .metadata.name "loadbalancer-config" -}}
	{{- render .metadata.name .data.template -}}
	{{- end -}}
	{{- end -}}'


**NOTE:** This runs with the permissions you give it, so be careful.
You can run it with limited service accounts and \*nix perms.

**TODO:**

* Cleanup and make nice and usable.
* Examples.
* Allow CRDs, and auto discovery.
* Read individual resources rather than watch them all.
* Implement UpdateStatus and Patching.
* Add webserver to add templated output for Prometheus
(or whatever).
* Implement some form of master election.
* Allow to store local states between template executions.
* Improve exec to allow piping in, timing, and indefinite
length runs(think running a permanent child process), chroot..
* Implement signal handling.
* Test in-cluster config.
* Test filtering with labels.

**Inspiration:**

* bash + kubectl -o go-template="" - very flexible, it can
do most of this.
* https://github.com/3cky/kube-template - almost does what I
wanted.
* Helm, it does part of what I want, but not all.

**Design Decisions:**
* Use go, because a single binary to use both inside and
outside the K8s cluster.
* Testing? LOL, what do you think I am? Either competent or
a coder? If you want to help, please educate me. :)
* Other crap, "because it works", I wrote a lot of things
differently because I thought it made sense, but it didn't
work!... Remember I'm clueless with go, so if you have a
better idea, please.... :)

