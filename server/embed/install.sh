#!/bin/sh

set -e

VERSION="v0.1.0"

if [ "$OS" = "Windows_NT" ]; then
  echo "Windows is not supported"
  exit 1
fi

case $(uname -sm) in
  "Darwin x86_64")
    target="darwin-amd64"
  ;;
  "Darwin arm64")
    target="darwin-arm64"
  ;;
  "Linux aarch64")
    target="linux-arm64"
  ;;
  *)
    target="linux-amd64"
  ;;
esac

dl_url="https://github.com/esm-dev/esm.sh/releases/download/${VERSION}/cli-${target}.gz"
bin_dir="$HOME/.esm.sh/bin"
exe="$bin_dir/esm.sh"

if [ ! -d "$bin_dir" ]; then
  mkdir -p "$bin_dir"
fi

curl --fail --location --progress-bar --output "$exe.gz" "$dl_url"
tar -xzf "$exe.gz" -C "$bin_dir"
chmod +x "$exe"
rm "$exe.gz"

shell_name=$(basename "$SHELL")
case $shell_name in
  fish)
    config_files="$HOME/.config/fish/config.fish"
  ;;
  zsh)
    config_files="$HOME/.zshrc $HOME/.zshenv $XDG_CONFIG_HOME/zsh/.zshrc $XDG_CONFIG_HOME/zsh/.zshenv"
  ;;
  bash)
    config_files="$HOME/.bashrc $HOME/.bash_profile $HOME/.profile $XDG_CONFIG_HOME/bash/.bashrc $XDG_CONFIG_HOME/bash/.bash_profile"
  ;;
  ash)
    config_files="$HOME/.ashrc $HOME/.profile /etc/profile"
  ;;
  sh)
    config_files="$HOME/.ashrc $HOME/.profile /etc/profile"
  ;;
  *)
    config_files="$HOME/.bashrc $HOME/.bash_profile $XDG_CONFIG_HOME/bash/.bashrc $XDG_CONFIG_HOME/bash/.bash_profile"
  ;;
esac

config_file=""
for file in $config_files; do
  if [[ -f $file ]]; then
    config_file=$file
    break
  fi
done

add_path() {
  if grep -Fxq "$1" "$config_file"; then
    echo "esm.sh CLI is already added to \$PATH in $config_file"
  elif [[ -w $config_file ]]; then
    echo -e "\n# esm.sh" >> "$config_file"
    echo "$1" >> "$config_file"
    echo "\033[32mSuccessfully added esm.sh CLI to \$PATH in $config_file\033[0m"
  else
    echo "Manually add the path to $config_file (or similar):"
    echo "\033[2m  $1\033[22m"
  fi
}

if [[ -z $config_file ]]; then
  echo "No config file found for $shell_name. You may need to manually add to PATH:"
  echo "\033[2m  export PATH=$bin_dir:\$PATH\033[22m"
elif [[ ":$PATH:" != *":$bin_dir:"* ]]; then
  case $shell_name in
    fish)
      add_path "fish_add_path $bin_dir"
    ;;
    zsh)
      add_path "export PATH=$bin_dir:\$PATH"
    ;;
    bash)
      add_path "export PATH=$bin_dir:\$PATH"
    ;;
    ash)
      add_path "export PATH=$bin_dir:\$PATH"
    ;;
    sh)
      add_path "export PATH=$bin_dir:\$PATH"
    ;;
    *)
      export PATH=$bin_dir:$PATH
      echo "Manually add the path to $config_file (or similar):"
      echo "\033[2m  export PATH=$bin_dir:\$PATH\033[22m"
    ;;
  esac
fi
