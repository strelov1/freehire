#!/usr/bin/env bash
set -euo pipefail
archive=$(mktemp)
extract_dir=$(mktemp -d)
trap 'rm -f "$archive"; rm -rf "$extract_dir"' EXIT
curl -fsSL --retry 5 --retry-all-errors https://github.com/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.linux.x86_64.tar.xz -o "$archive"
printf '%s  %s\n' 8c3be12b05d5c177a04c29e3c78ce89ac86f1595681cab149b65b97c4e227198 "$archive" | sha256sum --check --strict -
tar -xJf "$archive" -C "$extract_dir"
mkdir -p "$HOME/.local/bin"
install -m 0755 "$extract_dir/shellcheck-v0.11.0/shellcheck" "$HOME/.local/bin/shellcheck"
echo "$HOME/.local/bin" >> "$GITHUB_PATH"
