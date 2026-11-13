#!/bin/sh
# Install papercuts from the latest GitHub release.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/ryan-warner/papercuts/main/install.sh | sh
#
# The script installs to $HOME/.local/bin. Override with PAPERCUTS_INSTALL_DIR
# or pass a directory as the first argument.

set -eu

REPO="ryan-warner/papercuts"
INSTALL_DIR="${1:-${PAPERCUTS_INSTALL_DIR:-$HOME/.local/bin}}"

fail() {
  echo "install: $1" >&2
  exit 1
}

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported operating system $(uname -s); download a release from https://github.com/$REPO/releases" ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) fail "unsupported processor $(uname -m)" ;;
esac

command -v curl >/dev/null 2>&1 || fail "curl is required"

tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
  sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
  head -n 1)
[ -n "$tag" ] || fail "no release found; check https://github.com/$REPO/releases"

version=${tag#v}
archive="papercuts_${version}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

base_url="https://github.com/$REPO/releases/download/$tag"
curl -fsSL -o "$tmp/$archive" "$base_url/$archive" ||
  fail "download failed for $archive"
curl -fsSL -o "$tmp/checksums.txt" "$base_url/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  checksum_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  checksum_cmd="shasum -a 256"
else
  fail "sha256sum or shasum is required to verify the download"
fi
(cd "$tmp" && grep "  $archive\$" checksums.txt | $checksum_cmd -c -) ||
  fail "checksum verification failed for $archive"

tar -xzf "$tmp/$archive" -C "$tmp"
[ -f "$tmp/papercuts" ] || fail "archive did not contain a papercuts binary"

mkdir -p "$INSTALL_DIR"
chmod 755 "$tmp/papercuts"
cp -f "$tmp/papercuts" "$INSTALL_DIR/papercuts"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "note: $INSTALL_DIR is not on your PATH; add it to run 'papercuts' directly" >&2 ;;
esac

echo "installed papercuts $version to $INSTALL_DIR/papercuts"
"$INSTALL_DIR/papercuts" version

# Offer to append agent guidance. Prompts read from /dev/tty because stdin
# holds the piped script when run via curl | sh.
setup_agents() {
  bin="$INSTALL_DIR/papercuts"

  # Newer releases ship an interactive setup-agents command.
  if "$bin" help setup-agents >/dev/null 2>&1; then
    if true 2>/dev/null < /dev/tty; then
      "$bin" setup-agents < /dev/tty
    else
      "$bin" setup-agents
    fi
    return
  fi

  set -- "$HOME/.codex/AGENTS.md" \
    "$HOME/.claude/CLAUDE.md" \
    "$HOME/.config/opencode/AGENTS.md"

  present=false
  for file in "$@"; do
    [ -d "$(dirname "$file")" ] && present=true
  done
  if [ "$present" = false ]; then
    return 0
  fi

  # /dev/tty exists as a device node even without a controlling terminal;
  # probe whether it can be opened.
  if ! true 2>/dev/null < /dev/tty; then
    echo ""
    echo "next: teach your agents to record papercuts"
    for file in "$@"; do
      [ -d "$(dirname "$file")" ] || continue
      echo "  papercuts agent-instructions >> \"$file\""
    done
    return 0
  fi

  echo ""
  echo "agent setup"
  for file in "$@"; do
    [ -d "$(dirname "$file")" ] || continue
    if grep -q "papercuts" "$file" 2>/dev/null; then
      echo "  $file (already configured)"
      continue
    fi
    printf "  add papercuts guidance to %s? [y/N] " "$file"
    answer=""
    read answer < /dev/tty || answer=""
    answer=$(printf '%s' "$answer" | tr '[:upper:]' '[:lower:]')
    case "$answer" in
      y | yes)
        printf '\n' >> "$file"
        if "$bin" agent-instructions >> "$file"; then
          echo "  added guidance to $file"
        else
          echo "  failed to update $file" >&2
          exit 1
        fi
        ;;
    esac
  done
}
setup_agents

sign_off() {
  configured=false
  for file in "$HOME/.codex/AGENTS.md" "$HOME/.claude/CLAUDE.md" "$HOME/.config/opencode/AGENTS.md"; do
    if grep -q "papercuts" "$file" 2>/dev/null; then
      configured=true
      break
    fi
  done
  echo ""
  echo "all set. work as normal."
  if [ "$configured" = true ]; then
    echo "ask an agent to \"fix the papercuts\" whenever you're ready."
  else
    echo "log friction with papercuts add \"<observation>\", then run papercuts fix when you're ready."
  fi
}
sign_off
