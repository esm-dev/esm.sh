#!/bin/bash

init="no"
host="$1"
if [ "$host" == "--init" ]; then
  init="yes"
  host="$2"
fi

port="80"
tlsPort="0"
workDir="/etc/esmd"
cacheUrl=""
fsUrl=""
dbUrl=""
origin=""
npmRegistry=""
npmToken=""

if [ "$init" == "yes" ]; then
  echo "Server options:"
  read -p "? http server port (default is ${port}): " v
  if [ "$v" != "" ]; then
    port="$v"
  fi
  read -p "? https(autocert) server port (default is disabled): " v
  if [ "$v" != "" ]; then
    tlsPort="$v"
  fi
  read -p "? etc directory (ensure you have the r/w permission of it, default is '${workDir}'): " v
  if [ "$v" != "" ]; then
    workDir="$v"
  fi
  read -p "? cache config (default is 'memory:main'): " v
  if [ "$v" != "" ]; then
    cacheUrl="$v"
  fi
  read -p "? fs config (default is 'local:\$workDir/storage'): " v
  if [ "$v" != "" ]; then
    fsUrl="$v"
  fi
  read -p "? db config (default is 'postdb:\$workDir/esm.db'): " v
  if [ "$v" != "" ]; then
    dbUrl="$v"
  fi
  read -p "? server origin (optional): " v
  if [ "$v" != "" ]; then
    origin="$v"
  fi
  read -p "? npm registry (optional): " v
  if [ "$v" != "" ]; then
    npmRegistry="$v"
  fi
  read -p "? private token for npm registry (optional): " v
  if [ "$v" != "" ]; then
    npmToken="$v"
  fi
  echo "---"
fi

if [ "$host" == "" ]; then
  read -p "? deploy to (domain or IP): " v
  if [ "$v" != "" ]; then
    host="$v"
  fi
fi

if [ "$host" == "" ]; then
  echo "missing host"
  exit
fi

user="root"
read -p "? login user (default is 'root'): " v
if [ "$v" != "" ]; then
  user="$v"
fi

sshPort="22"
read -p "? ssh port (default is 22): " v
if [ "$v" != "" ]; then
  sshPort="$v"
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

  SVCF=/etc/supervisor/conf.d/esmd.conf
  writeSVConfLine () {
    echo "\$1" >> \$SVCF
  }

  supervisorctl stop esmd
  rm -f /usr/local/bin/esmd
  mv -f /tmp/esmd /usr/local/bin/esmd
  chmod +x /usr/local/bin/esmd

  if [ "$init" == "yes" ]; then
    if [ -f \$SVCF ]; then
      rm -f \$SVCF
    fi
    mkdir -p /etc/esmd
    echo "{\"port\":${port},\"tlsPort\":${tlsPort},\"workDir\":\"${workDir}\",\"cache\":\"${cacheUrl}\",\"storage\":\"${fsUrl}\",\"database\":\"${dbUrl}\",\"origin\":\"${origin}\",\"npmRegistry\":\"${npmRegistry}\",\"npmToken\":\"${npmToken}\"}" >> /etc/esmd/config.json
    writeSVConfLine "[program:esmd]"
    writeSVConfLine "command=/usr/local/bin/esmd --config=/etc/esmd/config.json"
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
