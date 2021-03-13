#!/bin/bash

goos="linux"
read -p "please enter the deploy GOOS(default is '$goos'): " val
if [ "$val" != "" ]; then
    goos="$val"
fi

goarch="amd64"
read -p "please enter the deploy GOARCH(default is '$goarch'): " val
if [ "$val" != "" ]; then
    goarch="$val"
fi

read -p "split China traffic ('yes' or 'no', default is 'no')? " p
if [ "$p" == "yes" ]; then
    go run $(dirname $0)/prebuild.go $(dirname $0)
    if [ "$?" != "0" ]; then
        exit
    fi
fi

echo "--- building(${goos}_$goarch)..."
export GOOS=$goos
export GOARCH=$goarch
go build -o $(dirname $0)/esmd $(dirname $0)/../main.go
