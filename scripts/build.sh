#!/bin/bash

goos="linux"
read -p "please enter the deploy OS(default is '$goos'): " val
if [ "$val" != "" ]; then
    goos="$val"
fi

goarch="amd64"
read -p "please enter the deploy CPU Architecture(default is '$goarch'): " val
if [ "$val" != "" ]; then
    goarch="$val"
fi

echo "--- prebuild..."
go run $(dirname $0)/prebuild.go $(dirname $0)
if [ "$?" != "0" ]; then
    exit
fi

echo "--- building(${goos}_$goarch)..."
export GOOS=$goos
export GOARCH=$goarch
go build -o esmd $(dirname $0)/../main.go
