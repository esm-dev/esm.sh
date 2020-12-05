#!/bin/bash

host="$1"
if [ "$host" == "" ]; then
    read -p "please enter the host address: " h
    if [ "$h" != "" ]; then
        host="$h"
    fi
fi

if [ "$host" == "" ]; then
    echo "invalid host"
    exit
fi

user="root"
read -p "please enter the ssh login user (default is 'root'): " u
if [ "$u" != "" ]; then
    user="$u"
fi

sshPort="22"
read -p "please enter the ssh port (default is 22): " p
if [ "$p" != "" ]; then
    sshPort="$p"
fi

init="no"
read -p "initiate service ('yes' or 'no', default is 'no')? " ok
if [ "$ok" == "yes" ]; then
    init="yes"
fi

rebuild="no"
buildVersion="1"
if [ "$init" == "no" ]; then
    read -p "rebuild ('yes' or 'no', default is 'no')? " ok
    if [ "$ok" == "yes" ]; then
        rebuild="yes"
        read -p "please enter the new builder id (default is 1): " p
        if [ "$p" != "" ]; then
            buildVersion="$p"
        fi
    fi
fi

port="80"
httpsPort="443"
etcDir="/etc/esmd"
domain="esm.sh"
cdnDomain=""
cdnDomainChina=""

if [ "$init" == "yes" ]; then
    read -p "please enter the server http port (default is ${port}): " p
    if [ "$p" != "" ]; then
        port="$p"
    fi
    read -p "please enter the server https port (default is ${httpsPort}): " p
    if [ "$p" != "" ]; then
        httpsPort="$p"
    fi
    read -p "please enter the etc directory, user ${user} must have r/w permission of it (default is ${etcDir}): " p
    if [ "$p" != "" ]; then
        etcDir="$p"
    fi
    read -p "please enter the server domain (default is ${domain}): " p
    if [ "$p" != "" ]; then
        domain="$p"
    fi
    read -p "please enter the cdn domain (optional): " p
    if [ "$p" != "" ]; then
        cdnDomain="$p"
    fi
    read -p "please enter the cdn domain for China (optional): " p
    if [ "$p" != "" ]; then
        cdnDomainChina="$p"
    fi
fi

if [ "$rebuild" == "yes" ]; then
    read -p "please enter the etc directory, user ${user} must have r/w permission of it (default is ${etcDir}): " p
    if [ "$p" != "" ]; then
        etcDir="$p"
    fi
fi

sh $(dirname $0)/build.sh
if [ "$?" != "0" ]; then
    exit
fi

echo "--- uploading..."
scp -P $sshPort esmd $user@$host:/tmp/esmd
if [ "$?" != "0" ]; then
    rm -f esmd
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
        if [ "$rebuild" == "yes" ]; then
            rm -f ${etcDir}/esm.db
            rm -rf ${etcDir}/storage
            echo "$buildVersion" > ${etcDir}/build.ver
            echo "esmd: rebuilt"
        fi
        supervisorctl start esmd
    fi
EOF

rm -f $(dirname $0)/../server/auto_mmdbr.go
rm -f $(dirname $0)/../server/auto_polyfills.go
rm -f $(dirname $0)/../server/auto_readme.go
rm -f $(dirname $0)/esmd
