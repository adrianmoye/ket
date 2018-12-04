#!/bin/bash -x

case "$1" in
	incontainer)
		export GOCACHE=$PWD/cache
		export GOPATH=$PWD
		export GOARCH=amd64
		export GOBIN=$PWD/bin
		mkdir -p $GOCACHE $GOBIN
		go get -v
		for GOOS in linux darwin;do
			export GOOS
			go build -i -v -o ket-$GOOS-$GOARCH || exit 1
		done
	;;
	shell)
		docker run \
			-it \
			--rm \
			-v "$PWD":/usr/src/ket \
			-w /usr/src/ket \
			golang \
			bash
	;;
	*)
		docker run \
			--rm \
			-v "$PWD":/usr/src/ket \
			-w /usr/src/ket \
			golang \
			./$0 incontainer
	;;
esac

