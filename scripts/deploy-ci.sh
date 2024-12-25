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
go build -ldflags="-s -w" -o esmd $(dirname $0)/../main.go
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

  servicefn=/etc/systemd/system/esmd.service
  reload=no
  if [ ! -f \$servicefn ]; then
    echo "[Unit]" >> \$servicefn
    echo "Description=esm.sh service" >> \$servicefn
    echo "After=network.target" >> \$servicefn
    echo "StartLimitIntervalSec=0" >> \$servicefn
    echo "[Service]" >> \$servicefn
    echo "Type=simple" >> \$servicefn
    echo "ExecStart=/usr/local/bin/esmd" >> \$servicefn
    echo "USER=\${USER}" >> \$servicefn
    echo "Restart=always" >> \$servicefn
    echo "RestartSec=5" >> \$servicefn
    echo "Environment=\"USER=\${USER}\"" >> \$servicefn
    echo "Environment=\"HOME=\${HOME}\"" >> \$servicefn
    echo "[Install]" >> \$servicefn
    echo "WantedBy=default.target" >> \$servicefn
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
