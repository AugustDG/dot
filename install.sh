#!/usr/bin/env bash
# Installs the dot CLI from the latest GitHub release, then runs `dot init`
# with any arguments given.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/AugustDG/dot/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/AugustDG/dot/main/install.sh | bash -s -- owner/dotfiles
#
# DOT_INSTALL_DIR sets where the binary goes (default ~/.local/bin).
# DOT_SKIP_INIT=1 installs the binary without running `dot init`.

set -euo pipefail

REPO="AugustDG/dot"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31mxx\033[0m  %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl is required"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux|darwin) ;;
  *)            die "Unsupported OS: $OS" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)             die "Unsupported architecture: $ARCH" ;;
esac

BINARY="dot-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/latest/${BINARY}"

INSTALL_DIR="${DOT_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$INSTALL_DIR"

log "Downloading $BINARY..."
# Temp file lives in INSTALL_DIR so the final mv is an atomic same-filesystem
# rename.
TMPFILE="$(mktemp "${INSTALL_DIR}/.dot.XXXXXX")"
trap 'rm -f "$TMPFILE"' EXIT
if ! curl -fsSL --retry 3 --connect-timeout 10 "$URL" -o "$TMPFILE"; then
  die "Failed to download from $URL"
fi
chmod +x "$TMPFILE"
mv "$TMPFILE" "${INSTALL_DIR}/dot"
log "Installed ${INSTALL_DIR}/dot"

[ "${DOT_SKIP_INIT:-}" = "1" ] && exit 0

export PATH="$INSTALL_DIR:$PATH"

log "Running dot init $*"
# Under `curl | bash`, stdin is the script pipe; reattach the terminal so the
# interactive module picker can run.
if ( : </dev/tty ) 2>/dev/null; then
  exec dot init "$@" </dev/tty
else
  exec dot init "$@"
fi
