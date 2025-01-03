#!/bin/bash

mkdir -p ~/.ssh
echo "${DEPLOY_SSH_PRIVATE_KEY}" >> ~/.ssh/id_ed25519
chmod 600 ~/.ssh/id_ed25519
ssh-keyscan $DEPLOY_HOST_NAME >> ~/.ssh/known_hosts
echo "Host d.esm.sh" >> ~/.ssh/config
echo "  HostName ${DEPLOY_HOST_NAME}" >> ~/.ssh/config
echo "  Port ${DEPLOY_HOST_PORT}" >> ~/.ssh/config
echo "  User ${DEPLOY_SSH_USER}" >> ~/.ssh/config
echo "  IdentityFile ~/.ssh/id_ed25519" >> ~/.ssh/config
echo "  IdentitiesOnly yes" >> ~/.ssh/config

echo "--- building server..."
go build -ldflags="-s -w -X 'github.com/esm-dev/esm.sh/server.VERSION=${SERVER_VERSION}'" -o esmd $(dirname $0)/../main.go
if [ "$?" != "0" ]; then
  exit 1
fi
du -h esmd

echo "--- uploading server build..."
tar -czf esmd.tar.gz esmd
scp esmd.tar.gz d.esm.sh:/tmp/esmd.tar.gz
if [ "$?" != "0" ]; then
  exit 1
fi

echo "--- installing server..."
ssh d.esm.sh << EOF
  gv=\$(git version)
  if [ "\$?" != "0" ]; then
    apt update
    apt install -y git
  fi
  echo \$gv

  configfile=/etc/esmd/config.json
  servicefile=/etc/systemd/system/esmd.service
  reload=no
  if [ ! -f \$servicefile ]; then
    addgroup esm
    adduser --ingroup esm --no-create-home --disabled-login --disabled-password --gecos "" esm
    if [ "\$?" != "0" ]; then
      echo "Failed to add user 'esm'"
      exit 1
    fi
    mkdir /etc/esmd
    chown esm:esm /etc/esmd
    echo "[Unit]" >> \$servicefile
    echo "Description=esm.sh service" >> \$servicefile
    echo "After=network.target" >> \$servicefile
    echo "StartLimitIntervalSec=0" >> \$servicefile
    echo "[Service]" >> \$servicefile
    echo "Type=simple" >> \$servicefile
    echo "ExecStart=/usr/local/bin/esmd --config=\$configfile" >> \$servicefile
    echo "WorkingDirectory=/esm" >> \$servicefile
    echo "USER=esm" >> \$servicefile
    echo "Restart=always" >> \$servicefile
    echo "RestartSec=5" >> \$servicefile
    echo "Environment=\"ESMDIR=/esm\"" >> \$servicefile
    echo "[Install]" >> \$servicefile
    echo "WantedBy=multi-user.target" >> \$servicefile
    reload=yes
  else
    systemctl stop esmd.service
    echo "Stopped esmd.service."
  fi

  rm -f \$configfile
  if [ "$SERVER_CONFIG" != "" ]; then
    echo "${SERVER_CONFIG}" >> \$configfile
  else
    echo "{}" >> \$configfile
  fi
  chown esm:esm \$configfile

  if [ "$RESET_ON_DEPLOY" == "yes" ]; then
    mkdir -p /tmp/.esm
    mv -f /esm/* /tmp/.esm
    nohup rm -rf /tmp/.esm &
  fi

  cd /tmp
  tar -xzf esmd.tar.gz
  if [ "\$?" != "0" ]; then
    exit 1
  fi
  rm -f esmd.tar.gz
  chmod +x esmd
  mv -f esmd /usr/local/bin/esmd

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
