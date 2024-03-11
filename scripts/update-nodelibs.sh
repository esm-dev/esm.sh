#!/bin/bash

tag="2.0.1"
dlUrl="https://codeload.github.com/jspm/jspm-core/tar.gz/refs/tags/${tag}"

curl -o "jspm-core-${tag}.tar.gz" $dlUrl
tar -xzf jspm-core-${tag}.tar.gz
cd jspm-core-${tag}/nodelibs
rm -rf node
mv browser node
tar -czf nodelibs.tar.gz node
mv nodelibs.tar.gz ../../server/embed/nodelibs.tar.gz
cd ../../
rm -rf jspm-core-${tag}
rm -rf jspm-core-${tag}.tar.gz
