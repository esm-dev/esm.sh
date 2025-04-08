#!/bin/bash

mkdir -p ~/.ssh
echo "${DEPLOY_SSH_PRIVATE_KEY}" >> ~/.ssh/id_ed25519
chmod 600 ~/.ssh/id_ed25519
ssh-keyscan $DEPLOY_HOST >> ~/.ssh/known_hosts
echo "Host esm.sh" >> ~/.ssh/config
echo "  HostName ${DEPLOY_HOST}" >> ~/.ssh/config
echo "  Port ${DEPLOY_SSH_PORT}" >> ~/.ssh/config
echo "  User ${DEPLOY_SSH_USER}" >> ~/.ssh/config
echo "  IdentityFile ~/.ssh/id_ed25519" >> ~/.ssh/config
echo "  IdentitiesOnly yes" >> ~/.ssh/config

echo "--- building server..."
go build -ldflags="-s -w -X 'github.com/esm-dev/esm.sh/server.VERSION=${SERVER_VERSION}'" -o esmd $(dirname $0)/../server/esmd/main.go
if [ "$?" != "0" ]; then
  exit 1
fi
du -h esmd

echo "--- uploading server build..."
tar -czf esmd.tar.gz esmd
scp esmd.tar.gz esm.sh:/tmp/esmd.tar.gz
if [ "$?" != "0" ]; then
  exit 1
fi

echo "--- installing server..."
ssh esm.sh << EOF
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
  reload=no

  if [ ! -f \$servicerc ]; then
    addgroup esm
    adduser --ingroup esm --home /esm --disabled-login --disabled-password --gecos "" esm
    echo "[Unit]" >> \$servicerc
    echo "Description=esm.sh service" >> \$servicerc
    echo "After=network.target" >> \$servicerc
    echo "StartLimitIntervalSec=0" >> \$servicerc
    echo "[Service]" >> \$servicerc
    echo "Type=simple" >> \$servicerc
    echo "ExecStart=/usr/local/bin/esmd --config=\$configjson" >> \$servicerc
    echo "WorkingDirectory=/esm" >> \$servicerc
    echo "Group=esm" >> \$servicerc
    echo "User=esm" >> \$servicerc
    echo "AmbientCapabilities=CAP_NET_BIND_SERVICE" >> \$servicerc
    echo "Restart=always" >> \$servicerc
    echo "RestartSec=5" >> \$servicerc
    echo "Environment=\"ESMDIR=/esm\"" >> \$servicerc
    echo "[Install]" >> \$servicerc
    echo "WantedBy=multi-user.target" >> \$servicerc
    reload=yes
  else
    systemctl stop esmd.service
    echo "Stopped esmd.service."
  fi

  mv -f esmd /usr/local/bin/esmd

  rm -f \$configjson
  mkdir -p /etc/esmd
  if [ "$SERVER_CONFIG" != "" ]; then
    echo '${SERVER_CONFIG}' >> \$configjson
  else
    echo "{}" >> \$configjson
  fi

  if [ "$RESET_ON_DEPLOY" == "yes" ]; then
    mkdir -p /tmp/.esm
    mv -f /esm/* /tmp/.esm
    nohup rm -rf /tmp/.esm &
  fi

  if [ "\$reload" == "yes" ]; then
    systemctl daemon-reload
    systemctl enable esmd.service
  fi

  systemctl start esmd.service
  echo "Started esmd.service."
EOF
if [ "$?" != "0" ]; then
  exit 1
fi
