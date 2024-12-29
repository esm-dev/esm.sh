#!/bin/bash

mkdir -p ~/.ssh
ssh-keyscan $DEPLOY_HOST_NAME >> ~/.ssh/known_hosts
echo "${DEPLOY_SSH_PRIVATE_KEY}" >> ~/.ssh/id_ed25519
chmod 600 ~/.ssh/id_ed25519
echo "Host next.esm.sh" >> ~/.ssh/config
echo "  HostName ${DEPLOY_HOST_NAME}" >> ~/.ssh/config
echo "  Port ${DEPLOY_HOST_PORT}" >> ~/.ssh/config
echo "  User ${DEPLOY_SSH_USER}" >> ~/.ssh/config
echo "  IdentityFile ~/.ssh/id_ed25519" >> ~/.ssh/config
echo "  IdentitiesOnly yes" >> ~/.ssh/config

echo "--- building..."
go build -ldflags="-s -w -X 'github.com/esm-dev/esm.sh/server.VERSION=${VERSION}'" -o esmd $(dirname $0)/../main.go
if [ "$?" != "0" ]; then
  exit 1
fi

echo "--- uploading..."
du -h esmd
tar -czf esmd.tar.gz esmd
scp esmd.tar.gz next.esm.sh:/tmp/esmd.tar.gz
if [ "$?" != "0" ]; then
  exit 1
fi

echo "--- installing..."
ssh next.esm.sh << EOF
  gv=\$(git version)
  if [ "\$?" != "0" ]; then
    apt update
    apt install -y git
  fi
  echo \$gv

  servicefile=/etc/systemd/system/esmd.service
  reload=no
  if [ ! -f \$servicefile ]; then
    echo "[Unit]" >> \$servicefile
    echo "Description=esm.sh service" >> \$servicefile
    echo "After=network.target" >> \$servicefile
    echo "StartLimitIntervalSec=0" >> \$servicefile
    echo "[Service]" >> \$servicefile
    echo "Type=simple" >> \$servicefile
    echo "ExecStart=/usr/local/bin/esmd" >> \$servicefile
    echo "USER=\${USER}" >> \$servicefile
    echo "Restart=always" >> \$servicefile
    echo "RestartSec=5" >> \$servicefile
    echo "Environment=\"USER=\${USER}\"" >> \$servicefile
    echo "Environment=\"HOME=\${HOME}\"" >> \$servicefile
    echo "[Install]" >> \$servicefile
    echo "WantedBy=default.target" >> \$servicefile
    reload=yes
  else
    systemctl stop esmd.service
    echo "Stopped esmd.service."
  fi

  mv -f ~/.esmd /tmp/.esmd
  nohup rm -rf /tmp/.esmd &

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
