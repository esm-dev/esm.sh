#!/bin/bash

go build -o esmd $(dirname $0)/../main.go
if [ "$?" != "0" ]; then
  exit 1
fi
du -h esmd

mkdir -p ~/.ssh
ssh-keyscan $SSH_HOST_NAME >> ~/.ssh/known_hosts
echo "${SSH_PRIVATE_KEY}" >> ~/.ssh/id_ed25519
chmod 600 ~/.ssh/id_ed25519
echo "Host next.esm.sh" >> ~/.ssh/config
echo "  HostName ${SSH_HOST_NAME}" >> ~/.ssh/config
echo "  User ${SSH_USER}" >> ~/.ssh/config
echo "  IdentityFile ~/.ssh/id_ed25519" >> ~/.ssh/config
echo "  IdentitiesOnly yes" >> ~/.ssh/config

tar -czf esmd.tar.gz esmd
scp esmd.tar.gz next.esm.sh:/tmp/esmd.tar.gz
if [ "$?" != "0" ]; then
  exit 1
fi

ssh next.esm.sh << EOF
  cd /tmp
  tar -xzf esmd.tar.gz
  if [ "\$?" != "0" ]; then
    exit 1
  fi
  rm -rf esmd.tar.gz

  svv=\$(supervisorctl version)
  if [ "\$?" != "0" ]; then
    apt update
    apt install -y supervisor git git-lfs
    git lfs install
    svv=\$(supervisorctl version)
  fi
  echo "supervisor \${svv}"

  svcf=/etc/supervisor/conf.d/esmd.conf
  reload=no
  if [ ! -f \$svcf ]; then
    echo "[program:esmd]" >> \$svcf
    echo "command=/usr/local/bin/esmd" >> \$svcf
    echo "environment=USER=\"\${USER}\",HOME=\"\${HOME}\"" >> \$svcf
    echo "user=\${USER}" >> \$svcf
    echo "directory=/tmp" >> \$svcf
    echo "autostart=true" >> \$svcf
    echo "autorestart=true" >> \$svcf
    reload=yes
  else
    supervisorctl stop esmd
  fi

  mv -f ~/.esmd /tmp/.esmd
  nohup rm -rf /tmp/.esmd &

  rm -f /usr/local/bin/esmd
  mv -f esmd /usr/local/bin/esmd
  chmod +x /usr/local/bin/esmd

  if [ "\$reload" == "yes" ]; then
    supervisorctl reload
  else
    supervisorctl start esmd
  fi
EOF
if [ "$?" != "0" ]; then
  exit 1
fi
