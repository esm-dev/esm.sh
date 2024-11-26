#!/bin/bash

init="no"
host="$1"
if [ "$host" == "--init" ]; then
  init="yes"
  host="$2"
fi

config=""
if [ "$init" == "yes" ]; then
  read -p "? server configuration (JSON): " v
  if [ "$v" != "" ]; then
    config="$v"
  fi
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

read -p "? host os (default is 'linux'): " goos
read -p "? host architecture (default is 'amd64'): " goarch
if [ "$goos" == "" ]; then
  goos="linux"
fi
if [ "$goarch" == "" ]; then
  goarch="amd64"
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

echo "--- building(${goos}_$goarch)..."
export GOOS=$goos
export GOARCH=$goarch
go build -o esmd $(dirname $0)/../main.go
if [ "$?" != "0" ]; then
  exit
fi

echo "--- uploading..."
tar -czf esmd.tar.gz esmd
scp -P $sshPort esmd.tar.gz $user@$host:/tmp/esmd.tar.gz
if [ "$?" != "0" ]; then
  rm -f esmd
  rm -f esmd.tar.gz
  exit
fi

echo "--- installing..."
ssh -p $sshPort ${user}@${host} << EOF
  if [ "$init" == "yes" ]; then
    apt update
    apt install -y git git-lfs supervisor
    git lfs install
    svcf=/etc/supervisor/conf.d/esmd.conf
    rm -f \$svcf
    echo "[program:esmd]" >> \$svcf
    if [ "$config" == "" ]; then
      echo "command=/usr/local/bin/esmd" >> \$svcf
    else
      mkdir -p /etc/esmd
      rm -f /etc/esmd/config.json
      echo "$config" >> /etc/esmd/config.json
      echo "command=/usr/local/bin/esmd --config=/etc/esmd/config.json" >> \$svcf
    fi
    echo "environment=USER=\"\${USER}\",HOME=\"\${HOME}\"" >> \$svcf
    echo "user=\${USER}" >> \$svcf
    echo "directory=/tmp" >> \$svcf
    echo "autostart=true" >> \$svcf
    echo "autorestart=true" >> \$svcf
  else
    SVV=\$(supervisorctl version)
    if [ "\$?" != "0" ]; then
      echo "error: supervisor not installed!"
      exit
    fi
    echo "supervisor \$SVV"
    supervisorctl stop esmd
  fi

  cd /tmp
  tar -xzf esmd.tar.gz
  rm -rf esmd.tar.gz

  rm -f /usr/local/bin/esmd
  mv -f esmd /usr/local/bin/esmd
  chmod +x /usr/local/bin/esmd

  if [ "$init" == "yes" ]; then
    supervisorctl reload
  else
    supervisorctl start esmd
  fi
EOF

rm -f esmd
rm -f esmd.tar.gz
