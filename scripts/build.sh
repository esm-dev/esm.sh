#!/bin/bash

read -p "build GOOS (default is 'linux'): " goos
read -p "build GOARCH (default is 'amd64'): " goarch
if [ "$goos" == "" ]; then
  goos="linux"
fi
if [ "$goarch" == "" ]; then
  goarch="amd64"
fi

echo "--- building(${goos}_$goarch)..."
export GOOS=$goos
export GOARCH=$goarch
go build -o $(dirname $0)/esmd $(dirname $0)/../main.go
