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
go build -ldflags="-s -w" -o esmd $(dirname $0)/../server/esmd/main.go
if [ "$?" != "0" ]; then
  exit
fi
du -h esmd

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
  cd /tmp
  rm -f esmd
  tar -xzf esmd.tar.gz
  chmod +x esmd
  if [ "\$?" != "0" ]; then
    exit 1
  fi
  rm -f esmd.tar.gz

  git version
  if [ "\$?" == "127" ]; then
    apt-get update
    apt-get install -y git
  fi

  ufw version
  if [ "\$?" == "0" ]; then
    ufw allow http
    ufw allow https
  fi

  configjson=/etc/esmd/config.json
  servicerc=/etc/systemd/system/esmd.service

  if [ "$init" == "yes" ]; then
    addgroup esm
    adduser --ingroup esm --home=/esm --disabled-login --disabled-password --gecos "" esm
    rm -f \$servicerc
    echo "[Unit]" >> \$servicerc
    echo "Description=esm.sh service" >> \$servicerc
    echo "After=network.target" >> \$servicerc
    echo "StartLimitIntervalSec=0" >> \$servicerc
    echo "[Service]" >> \$servicerc
    echo "Type=simple" >> \$servicerc
    if [ "$config" != "" ]; then
      rm -f \$configjson
      mkdir -p /etc/esmd
      echo '$config' >> \$configjson
      echo "ExecStart=/usr/local/bin/esmd --config=\$configjson" >> \$servicerc
    else
      echo "ExecStart=/usr/local/bin/esmd" >> \$servicerc
    fi
    echo "WorkingDirectory=/esm" >> \$servicerc
    echo "Group=esm" >> \$servicerc
    echo "User=esm" >> \$servicerc
    echo "AmbientCapabilities=CAP_NET_BIND_SERVICE" >> \$servicerc
    echo "Restart=always" >> \$servicerc
    echo "RestartSec=5" >> \$servicerc
    echo "Environment=\"ESMDIR=/esm\"" >> \$servicerc
    echo "[Install]" >> \$servicerc
    echo "WantedBy=multi-user.target" >> \$servicerc
  else
    systemctl stop esmd.service
    echo "Stopped esmd.service."
  fi

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
