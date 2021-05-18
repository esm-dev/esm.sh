#!/bin/bash

# mmdb_china_ip_list: https://github.com/alecthw/mmdb_china_ip_list
mmdb_china_ip_list_tag="20210517"
dataUrl="https://github.com/alecthw/mmdb_china_ip_list/releases/download/${mmdb_china_ip_list_tag}/china_ip_list.mmdb"
saveAs="$(dirname $0)/../embed/china_ip_list.mmdb"
cacheTo="/tmp/china_ip_list.${mmdb_china_ip_list_tag}.mmdb"

read -p "split China traffic? y/N " split_china_traffic
read -p "build GOOS (default is 'linux'): " goos
read -p "build GOARCH (default is 'amd64'): " goarch

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
