#!/bin/bash

init="no"
host="$1"
if [ "$host" == "--init" ]; then
  init="yes"
  host="$2"
fi

port="80"
tlsPort="0"
workDir=""

if [ "$init" == "yes" ]; then
  echo "Server configuration:"
  read -p "? http server port (default is 80): " v
  if [ "$v" != "" ]; then
    port="$v"
  fi
  read -p "? enable https (y/N): " v
  if [ "$v" == "y" ]; then
    tlsPort="443"
  fi
  read -p "? workDir (ensure the user have the r/w permission of it, default is '~/.esmd'): " v
  if [ "$v" != "" ]; then
    workDir="$v"
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

read -p "? build GOOS (default is 'linux'): " goos
read -p "? build GOARCH (default is 'amd64'): " goarch
if [ "$goos" == "" ]; then
  goos="linux"
fi
if [ "$goarch" == "" ]; then
  goarch="amd64"
fi

echo "--- building(${goos}_$goarch)..."
export GOOS=$goos
export GOARCH=$goarch
go build -o $(dirname $0)/esmd $(dirname $0)/../main.go
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
    apt install -y supervisor git git-lfs
  fi

  SVV=\$(supervisorctl version)
  if [ "\$?" != "0" ]; then
    echo "error: supervisor not installed!"
    exit
  fi
  echo "supervisor \$SVV"

  SVCONF=/etc/supervisor/conf.d/esmd.conf
  writeSVConfLine () {
    echo "\$1" >> \$SVCONF
  }

  cd /tmp
  tar -xzf esmd.tar.gz
  rm -rf esmd.tar.gz

  supervisorctl stop esmd
  rm -f /usr/local/bin/esmd
  mv -f esmd /usr/local/bin/esmd
  chmod +x /usr/local/bin/esmd

  if [ "$init" == "yes" ]; then
    echo "fs.inotify.max_user_watches=524288" | sudo tee -a /etc/sysctl.conf
    sudo sysctl -p
    git lfs install
    if [ -f \$SVCONF ]; then
      rm -f \$SVCONF
    fi
    wd=$workDir
    if [ "$wd" == "" ]; then
      wd=~/.esmd
    fi
    mkdir $wd
    echo "{\"port\": ${port}, \"tlsPort\": ${tlsPort}, \"workDir\": \"${wd}\"}" >> /etc/esmd/config.json
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

rm -f esmd
rm -f esmd.tar.gz
