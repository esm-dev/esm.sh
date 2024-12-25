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

read -p "? host os (default is \"linux\"): " goos
read -p "? host architecture (default is \"amd64\"): " goarch
if [ "$goos" == "" ]; then
  goos="linux"
fi
if [ "$goarch" == "" ]; then
  goarch="amd64"
fi

user="root"
read -p "? login user (default is \"root\"): " v
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
go build -ldflags="-s -w" -o esmd $(dirname $0)/../main.go
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
  gv=\$(git version)
  if [ "\$?" != "0" ]; then
    apt update
    apt install -y git
  fi
  echo \$gv

  if [ "$init" == "yes" ]; then
    servicefn=/etc/systemd/system/esmd.service
    if [ -f \$servicefn ]; then
      rm -f servicefn
    fi
    echo "[Unit]" >> \$servicefn
    echo "Description=esm.sh service" >> \$servicefn
    echo "After=network.target" >> \$servicefn
    echo "StartLimitIntervalSec=0" >> \$servicefn
    echo "[Service]" >> \$servicefn
    echo "Type=simple" >> \$servicefn
    if [ "$config" != "" ]; then
      mkdir -p /etc/esmd
      rm -f /etc/esmd/config.json
      echo "$config" >> /etc/esmd/config.json
      echo "ExecStart=/usr/local/bin/esmd --config=/etc/esmd/config.json" >> \$servicefn
    else
      echo "ExecStart=/usr/local/bin/esmd" >> \$servicefn
    fi
    echo "USER=\${USER}" >> \$servicefn
    echo "Restart=always" >> \$servicefn
    echo "RestartSec=5" >> \$servicefn
    echo "Environment=\"USER=\${USER}\"" >> \$servicefn
    echo "Environment=\"HOME=\${HOME}\"" >> \$servicefn
    echo "[Install]" >> \$servicefn
    echo "WantedBy=default.target" >> \$servicefn
  else
    systemctl stop esmd.service
    echo "Stopped esmd.service."
  fi

  cd /tmp
  tar -xzf esmd.tar.gz
  if [ "\$?" != "0" ]; then
    exit 1
  fi
  rm -f esmd.tar.gz
  chmod +x esmd
  mv -f esmd /usr/local/bin/esmd

  if [ "$init" == "yes" ]; then
    systemctl daemon-reload
    systemctl enable esmd.service
  fi

  systemctl start esmd.service
  echo "Started esmd.service."
EOF

rm -f esmd
rm -f esmd.tar.gz
