#!/usr/bin/env bash
set -euo pipefail

# Runner images use this same mirror+file URI in Ubuntu 26.04's deb822 file
# and older runners' sources.list. Keep suites, components and trust unchanged.
apt_root="${CI_APT_ROOT:-/etc/apt}"
configured=false
for source in "$apt_root/sources.list.d/ubuntu.sources" "$apt_root/sources.list"; do
  if [[ -f "$source" ]] && grep -Fq 'mirror+file:/etc/apt/apt-mirrors.txt' "$source"; then
    configured=true
    break
  fi
done
if [[ "$configured" != true || ! -f "$apt_root/apt-mirrors.txt" ]]; then
  echo 'Expected the GitHub Ubuntu runner mirror+file APT layout' >&2
  exit 1
fi

# Explicit priorities matter: APT otherwise randomizes mirror selection. The
# independent host avoids the observed DNS failures across ubuntu.com mirrors.
printf '%s\tpriority:%s\n' \
  'https://mirrors.edge.kernel.org/ubuntu/' 1 \
  'https://archive.ubuntu.com/ubuntu/' 2 \
  'https://security.ubuntu.com/ubuntu/' 3 > "$apt_root/apt-mirrors.txt"
mkdir -p "$apt_root/apt.conf.d"
cat > "$apt_root/apt.conf.d/80-ci-mirrors" <<'EOF'
Acquire::Retries "1";
Acquire::http::Timeout "15";
Acquire::https::Timeout "15";
APT::Update::Error-Mode "any";
EOF
