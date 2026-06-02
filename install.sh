#!/bin/sh
# DevEdu Code installer.
#
#   curl -fsSL https://raw.githubusercontent.com/DevEdu-IO/code/main/install.sh | sh
#
# Detects your OS/arch, downloads the matching `devedu` binary from the latest
# GitHub Release, verifies its checksum, and installs it onto your PATH.
#
# Overrides:
#   INSTALL_DIR=~/.local/bin   where to install (default: /usr/local/bin)
#   VERSION=v1.2.0             a specific release tag (default: latest)
set -eu

REPO="DevEdu-IO/code"
BINARY="devedu"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"

err() { printf '%s\n' "$*" >&2; exit 1; }

# --- detect platform -------------------------------------------------------
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux)  os=linux ;;
  darwin) os=darwin ;;
  *) err "Unsupported OS '$os'. On Windows, download devedu-windows-amd64.exe from https://github.com/$REPO/releases" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "Unsupported architecture '$arch'." ;;
esac

asset="${BINARY}-${os}-${arch}"

# --- download URLs ---------------------------------------------------------
if [ "$VERSION" = "latest" ]; then
  base="https://github.com/${REPO}/releases/latest/download"
else
  base="https://github.com/${REPO}/releases/download/${VERSION}"
fi

# fetch <url> <output> — use curl or wget, whichever exists.
fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    err "Need curl or wget installed."
  fi
}

tmp=$(mktemp)
trap 'rm -f "$tmp" "$tmp.sums"' EXIT

printf 'Downloading %s (%s)…\n' "$asset" "$VERSION"
fetch "${base}/${asset}" "$tmp" || err "Download failed. Has a release been published yet? https://github.com/$REPO/releases"

# --- verify checksum (best effort: only if tools + sums file available) ----
sha=""
if command -v sha256sum >/dev/null 2>&1; then sha="sha256sum"
elif command -v shasum >/dev/null 2>&1; then sha="shasum -a 256"
fi
if [ -n "$sha" ] && fetch "${base}/SHA256SUMS.txt" "$tmp.sums" 2>/dev/null; then
  expected=$(grep " ${asset}\$" "$tmp.sums" 2>/dev/null | awk '{print $1}' || true)
  if [ -n "$expected" ]; then
    actual=$($sha "$tmp" | awk '{print $1}')
    [ "$expected" = "$actual" ] || err "Checksum mismatch for ${asset} — refusing to install."
    printf 'Checksum verified.\n'
  fi
fi

chmod +x "$tmp"

# --- install ---------------------------------------------------------------
dest="${INSTALL_DIR}/${BINARY}"
if mkdir -p "$INSTALL_DIR" 2>/dev/null && [ -w "$INSTALL_DIR" ]; then
  mv "$tmp" "$dest"
else
  printf 'Installing to %s (requires sudo)…\n' "$dest"
  sudo mkdir -p "$INSTALL_DIR"
  sudo mv "$tmp" "$dest"
  sudo chmod +x "$dest"
fi
trap - EXIT

printf '\nInstalled %s\n' "$dest"
"$dest" --version 2>/dev/null || true

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) printf "Run 'devedu' to get started.\n" ;;
  *) printf "\nNote: %s is not on your PATH. Add it, e.g.:\n  export PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR" "$INSTALL_DIR" ;;
esac
