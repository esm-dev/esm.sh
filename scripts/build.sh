#!/bin/bash

mmdb_china_ip_list_tag="20210308"
dataUrl="https://github.com/alecthw/mmdb_china_ip_list/releases/download/$mmdb_china_ip_list_tag/china_ip_list.mmdb"
saveAs="$(dirname $0)/../assets/china_ip_list.mmdb"

read -p "split China traffic y/N? " split_china_traffic
read -p "build GOOS (default is 'linux'): " goos
read -p "build GOARCH (default is 'amd64'): " goarch

if [ "$split_china_traffic" == "y" ]; then
  if [ ! -f "$saveAs" ]; then
    echo "--- download china_ip_list.mmdb..."
    curl --fail --location --progress-bar --output "$saveAs" "$dataUrl"
    if [ "$?" != "0" ]; then
      exit
    fi
  fi
else
  if [ -f "$saveAs" ]; then
    rm "$saveAs"
  fi
fi

if [ "$goos" == "" ]; then
  goos="linux"
fi
if [ "$goarch" == "" ]; then
  goarch="amd64"
fi
export GOOS=$goos
export GOARCH=$goarch
echo "--- building(${goos}_$goarch)..."
go build -o $(dirname $0)/esmd $(dirname $0)/../main.go
