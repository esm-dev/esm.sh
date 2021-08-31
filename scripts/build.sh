#!/bin/bash

# mmdb_china_ip_list: https://github.com/alecthw/mmdb_china_ip_list
mmdb_china_ip_list_tag="202108181710"
dataUrl="https://github.com/alecthw/mmdb_china_ip_list/releases/download/${mmdb_china_ip_list_tag}/china_ip_list.mmdb"
saveAs="$(dirname $0)/../embed/china_ip_list.mmdb"
cacheTo="/tmp/china_ip_list.${mmdb_china_ip_list_tag}.mmdb"

read -p "split China traffic? y/N " split_china_traffic
if [ "$split_china_traffic" == "y" ]; then
  if [ ! -f "$cacheTo" ]; then
    echo "--- download china_ip_list.mmdb..."
    curl --fail --location --progress-bar --output "$cacheTo" "$dataUrl"
    if [ "$?" != "0" ]; then
      exit
    fi
  fi
  cp -f $cacheTo $saveAs
else
  if [ -f "$saveAs" ]; then
    rm "$saveAs"
  fi
fi

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
