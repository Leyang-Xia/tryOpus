#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MIN_GO_MAJOR=1
MIN_GO_MINOR=22
DEFAULT_GO_VERSION="1.22.2"

version_ok() {
  local v="$1"
  v="${v#go}"
  local major minor
  major="${v%%.*}"
  minor="${v#*.}"
  minor="${minor%%.*}"
  if [[ -z "$major" || -z "$minor" ]]; then
    return 1
  fi
  if (( major > MIN_GO_MAJOR )); then
    return 0
  fi
  if (( major == MIN_GO_MAJOR && minor >= MIN_GO_MINOR )); then
    return 0
  fi
  return 1
}

ensure_go() {
  local goversion=""
  if command -v go >/dev/null 2>&1; then
    goversion="$(go env GOVERSION 2>/dev/null || true)"
    if [[ -z "$goversion" ]]; then
      goversion="$(go version | awk '{print $3}')"
    fi
    if version_ok "$goversion"; then
      echo "[cursor-cloud] Found $goversion (>= go1.22), skip install."
      return 0
    fi
    echo "[cursor-cloud] Found $goversion (< go1.22), installing newer Go..."
  else
    echo "[cursor-cloud] Go not found, installing Go..."
  fi

  local go_version="${GO_VERSION:-$DEFAULT_GO_VERSION}"
  local uname_arch
  uname_arch="$(uname -m)"
  local go_arch
  case "$uname_arch" in
    x86_64) go_arch="amd64" ;;
    aarch64|arm64) go_arch="arm64" ;;
    *)
      echo "[cursor-cloud] Unsupported architecture: $uname_arch"
      return 1
      ;;
  esac

  local install_dir="$HOME/.local/go-$go_version"
  local go_root="$install_dir/go"
  local tarball="go${go_version}.linux-${go_arch}.tar.gz"
  local url="https://go.dev/dl/${tarball}"

  rm -rf "$install_dir"
  mkdir -p "$install_dir"
  echo "[cursor-cloud] Downloading ${url}"
  curl -fsSL "$url" -o "$install_dir/$tarball"
  tar -C "$install_dir" -xzf "$install_dir/$tarball"
  rm -f "$install_dir/$tarball"

  export PATH="$go_root/bin:$PATH"
  echo "[cursor-cloud] Installed $(go version) at $go_root"
}

warm_webrtc_demo_mod() {
  local demo_dir="$ROOT_DIR/webrtc_demo"
  if [[ ! -f "$demo_dir/go.mod" ]]; then
    echo "[cursor-cloud] Missing $demo_dir/go.mod, skip prewarm."
    return 0
  fi
  echo "[cursor-cloud] Running go mod download in $demo_dir"
  (
    cd "$demo_dir"
    go mod download
  )
  echo "[cursor-cloud] webrtc_demo dependencies prewarmed."
}

ensure_go
warm_webrtc_demo_mod
