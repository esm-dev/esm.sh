#!/bin/bash

host="$1"
if [ "$host" == "" ]; then
    read -p "please enter the server hostname or ip to deploy: " h
    if [ "$h" != "" ]; then
        host="$h"
    fi
fi

if [ "$host" == "" ]; then
    echo "invalid server"
    exit
fi

loginUser="root"
read -p "please enter the host ssh login user (default is 'root'): " user
if [ "$user" != "" ]; then
    loginUser="$user"
fi

hostSSHPort="22"
read -p "please enter the host ssh port (default is 22): " port
if [ "$port" != "" ]; then
    hostSSHPort="$port"
fi

init="no"
read -p "initiate services ('yes' or 'no', default is 'no')? " ok
if [ "$ok" == "yes" ]; then
    init="yes"
fi

port="80"
httpsPort="443"
if [ "$init" == "yes" ]; then
    read -p "please enter the server http port (default is 80): " p
    if [ "$p" != "" ]; then
        port="$p"
    fi
    read -p "please enter the server https port (default is 443): " p
    if [ "$p" != "" ]; then
        httpsPort="$p"
    fi
fi

sh $(dirname $0)/build.sh
if [ "$?" != "0" ]; then 
    exit
fi

echo "--- uploading..."
scp -P $hostSSHPort esmsh $loginUser@$host:/tmp/esmsh
if [ "$?" != "0" ]; then
    rm esmsh
    exit
fi

echo "--- installing..."
ssh -p $hostSSHPort $loginUser@$host << EOF
    supervisorctl status esmsh
    if [ "$?" != "0" ]; then
        echo "error: missing supervisor!"
        exit
    fi

    writeSVConfLine () {
        echo "\$1" >> /etc/supervisor/conf.d/esmsh.conf
    }

    supervisorctl stop esmsh
    rm -f /usr/local/bin/esmsh
    mv -f /tmp/esmsh /usr/local/bin/esmsh
    chmod +x /usr/local/bin/esmsh

    if [ "$init" == "yes" ]; then
        rm -f /etc/supervisor/conf.d/esmsh.conf
        writeSVConfLine "[program:esmsh]"
        writeSVConfLine "command=/usr/local/bin/esmsh -port=${port} -https-port=${httpsPort}"
        writeSVConfLine "directory=/tmp"
        writeSVConfLine "user=$loginUser"
        writeSVConfLine "autostart=true"
        writeSVConfLine "autorestart=true"        
        supervisorctl reload
    else
        supervisorctl start esmsh
    fi
EOF

rm -f esmsh
