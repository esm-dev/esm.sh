#!/bin/bash

tag="0.0.1"
dlUrl="https://codeload.github.com/esm-dev/esm-npm-polyfills/tar.gz/refs/tags/${tag}"

cd $(dirname $0)
curl -o "esm-npm-polyfills-${tag}.tar.gz" $dlUrl
mv "esm-npm-polyfills-${tag}.tar.gz" ../server/embed/npm-polyfills.tar.gz
