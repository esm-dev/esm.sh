#!/bin/bash

goos="linux"
read -p "please enter the deploy OS(default is '$goos'): " sys
if [ "$sys" != "" ]; then
    goos="$sys"
fi

goarch="amd64"
read -p "please enter the deploy CPU Architecture(default is '$goarch'): " arch
if [ "$arch" != "" ]; then
    goarch="$arch"
fi

echo "--- compiling(${goos}_$goarch)..."
export GOOS=$goos
export GOARCH=$goarch
go build -o esmsh ../main.go
