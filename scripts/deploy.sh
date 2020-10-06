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
if [ "$init" == "no" ]; then
    read -p "rebuild database ('yes' or 'no', default is 'no')? " ok
    if [ "$ok" == "yes" ]; then
        rebuild="yes"
    fi
fi

port="80"
httpsPort="443"
etcDir="/etc/esmd"
domain="esm.sh"
cdnDomain=""

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
        mkdir ${etcDir}
        rm -f /etc/supervisor/conf.d/esmd.conf
        writeSVConfLine "[program:esmd]"
        writeSVConfLine "command=/usr/local/bin/esmd --port=${port} --https-port=${httpsPort} --etc-dir=${etcDir} --domain=${domain} --cdn-domain=${cdnDomain}"
        writeSVConfLine "directory=/tmp"
        writeSVConfLine "user=$user"
        writeSVConfLine "autostart=true"
        writeSVConfLine "autorestart=true"
        supervisorctl reload
    else
        if [ "$rebuild" == "yes" ]; then
            rm -f ${etcDir}/esm.db
            rm -rf ${etcDir}/storage
            echo "esmd: database rebuilt"
        fi
        supervisorctl start esmd
    fi
EOF

rm -f server/readme_md.go
rm -f esmd
