#!/bin/bash

tag="2.0.1"
dlUrl="https://codeload.github.com/jspm/jspm-core/tar.gz/refs/tags/${tag}"

cd $(dirname $0)
curl -o "jspm-core-${tag}.tar.gz" $dlUrl
tar -xzf jspm-core-${tag}.tar.gz
mv  jspm-core-${tag}/nodelibs/browser node
tar -czf node-libs.tar.gz node
mv node-libs.tar.gz ../server/embed/node-libs.tar.gz
rm -rf jspm-core-${tag}
rm -rf node
rm -f jspm-core-${tag}.tar.gz
