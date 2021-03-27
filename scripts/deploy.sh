#!/bin/bash

host="$1"
if [ "$host" == "" ]; then
    read -p "deploy to (domain or IP): " h
    if [ "$h" != "" ]; then
        host="$h"
    fi
fi

if [ "$host" == "" ]; then
    echo "missing host"
    exit
fi

user="root"
read -p "login user (default is 'root'): " u
if [ "$u" != "" ]; then
    user="$u"
fi

sshPort="22"
read -p "ssh port (default is 22): " p
if [ "$p" != "" ]; then
    sshPort="$p"
fi

init="no"
read -p "initiate supervisor service? y/N " ok
if [ "$ok" == "y" ]; then
    init="yes"
fi

read -p "build version: " ver
if [ "$ver" == "" ]; then
    echo "missing build version!"
    exit
fi
read -p "repeat the build version: " ver2
if [ "$ver" != "$ver2" ]; then
    echo "build version not matched!"
    exit
fi
echo "$ver" > $(dirname $0)/../assets/build.ver

port="80"
httpsPort="443"
etcDir="/etc/esmd"
domain="esm.sh"
cdnDomain=""
cdnDomainChina=""

if [ "$init" == "yes" ]; then
    read -p "server http port (default is ${port}): " p
    if [ "$p" != "" ]; then
        port="$p"
    fi
    read -p "server https port (default is ${httpsPort}): " p
    if [ "$p" != "" ]; then
        httpsPort="$p"
    fi
    read -p "etc directory (user '${user}' must have the r/w permission of it, default is ${etcDir}): " p
    if [ "$p" != "" ]; then
        etcDir="$p"
    fi
    read -p "server domain (default is ${domain}): " p
    if [ "$p" != "" ]; then
        domain="$p"
    fi
    read -p "cdn domain (optional): " p
    if [ "$p" != "" ]; then
        cdnDomain="$p"
    fi
    read -p "cdn domain for China (optional): " p
    if [ "$p" != "" ]; then
        cdnDomainChina="$p"
    fi
fi

scriptsDir=$(dirname $0)
sh $scriptsDir/build.sh

if [ "$?" != "0" ]; then
    exit
fi

echo "--- uploading..."
scp -P $sshPort $scriptsDir/esmd $user@$host:/tmp/esmd
if [ "$?" != "0" ]; then
    rm -f $scriptsDir/esmd
    exit
fi

echo "--- installing..."
ssh -p $sshPort $user@$host << EOF
    SVVer=\$(supervisorctl version)
    if [ "\$?" != "0" ]; then
        echo "error: missing supervisor!"
        exit
    fi
    echo "supervisor \$SVVer"

    writeSVConfLine () {
        echo "\$1" >> /etc/supervisor/conf.d/esmd.conf
    }

    supervisorctl stop esmd
    rm -f /usr/local/bin/esmd
    mv -f /tmp/esmd /usr/local/bin/esmd
    chmod +x /usr/local/bin/esmd

    if [ "$init" == "yes" ]; then
        if [ -d "${etcDir}" ]; then
            rm -f ${etcDir}/esm.db
            rm -rf ${etcDir}/storage
        else
            mkdir ${etcDir}
        fi
        rm -f /etc/supervisor/conf.d/esmd.conf
        writeSVConfLine "[program:esmd]"
        writeSVConfLine "command=/usr/local/bin/esmd --port=${port} --https-port=${httpsPort} --etc-dir=${etcDir} --domain=${domain} --cdn-domain=${cdnDomain} --cdn-domain-china=${cdnDomainChina}"
        writeSVConfLine "directory=/tmp"
        writeSVConfLine "user=$user"
        writeSVConfLine "autostart=true"
        writeSVConfLine "autorestart=true"
        supervisorctl reload
    else
        supervisorctl start esmd
    fi
EOF

rm -f $scriptsDir/esmd
