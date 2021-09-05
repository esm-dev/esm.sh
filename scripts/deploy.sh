#!/bin/bash

host="$1"
if [ "$host" == "" ]; then
  read -p "deploy to (domain or IP): " v
  if [ "$v" != "" ]; then
    host="$v"
  fi
fi

if [ "$host" == "" ]; then
  echo "missing host"
  exit
fi

user="root"
read -p "login user (default is 'root'): " v
if [ "$v" != "" ]; then
  user="$v"
fi

sshPort="22"
read -p "ssh port (default is 22): " v
if [ "$v" != "" ]; then
  sshPort="$v"
fi

init="no"
read -p "initiate supervisor service? y/N " v
if [ "$v" == "y" ]; then
  init="yes"
fi

port="80"
httpsPort="0"
etcDir="/usr/local/etc/esmd"
fsUrl=""
dbUrl=""
cdnDomain=""
cdnDomainChina=""
unpkgDomain=""

if [ "$init" == "yes" ]; then
  read -p "http server  port (default is ${port}): " v
  if [ "$v" != "" ]; then
    port="$v"
  fi
  read -p "https(autotls) server port (default is disabled): " v
  if [ "$v" != "" ]; then
    httpsPort="$v"
  fi
  read -p "etc directory (ensure user '${user}' have the r/w permission of it, default is '${etcDir}'): " v
  if [ "$v" != "" ]; then
    etcDir="$v"
  fi
	read -p "fs config (default is 'local:${etcDir}/storage'): " v
  if [ "$v" != "" ]; then
    fsUrl="$v"
  fi
	read -p "db config (default is 'postdb:${etcDir}/esm.db'): " v
  if [ "$v" != "" ]; then
    dbUrl="$v"
  fi
  read -p "cdn domain (optional): " v
  if [ "$v" != "" ]; then
    cdnDomain="$v"
  fi
  read -p "cdn domain for China (optional): " v
  if [ "$v" != "" ]; then
    cdnDomainChina="$v"
  fi
  read -p "proxy domain for unpkg.com (optional): " v
  if [ "$v" != "" ]; then
    unpkgDomain="$v"
  fi
fi

scriptsDir=$(dirname $0)
sh $scriptsDir/build.sh

if [ "$?" != "0" ]; then
  exit
fi

cleanYarnCache="no"
read -p "clean yarn cache? y/N " v
if [ "$v" == "y" ]; then
  cleanYarnCache="yes"
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

  SVCF=/etc/supervisor/conf.d/esmd.conf
  writeSVConfLine () {
    echo "\$1" >> \$SVCF
  }

  supervisorctl stop esmd
  rm -f /usr/local/bin/esmd
  mv -f /tmp/esmd /usr/local/bin/esmd
  chmod +x /usr/local/bin/esmd

  if [ "$cleanYarnCache" == "yes" ]; then
    rm -rf /usr/local/share/.cache/yarn
  fi

  if [ "$init" == "yes" ]; then
    if [ -f \$SVCF ]; then
      rm -f \$SVCF
    fi
    writeSVConfLine "[program:esmd]"
    writeSVConfLine "command=/usr/local/bin/esmd --port=${port} --https-port=${httpsPort} --etc-dir=${etcDir} --fs=${fsUrl} --db=${dbUrl} --cdn-domain=${cdnDomain} --cdn-domain-china=${cdnDomainChina} --unpkg-domain=${unpkgDomain}"
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
