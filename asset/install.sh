#!/usr/bin/env bash

if [ -z "${BASH_VERSION:-}" ]; then
  echo "[ERROR] This installer must be run with bash." >&2
  echo "[ERROR] Try: curl -kfsSL http://cnpai-cnp-bdtest.gwmit.cn/install.sh | bash" >&2
  exit 1
fi

set -eu
set -o pipefail 2>/dev/null || true

PRODUCT_NAME="CNP-AI"
ARCHIVE_PREFIX="cnpai"
DEFAULT_VERSION="latest"
DEFAULT_BASE_URL="http://cnpai-cnp-bdtest.gwmit.cn"

BIN_NAME_IN_ARCHIVE="cnp-ai"
DEFAULT_BIN_DIR="/usr/local/bin"
TARGET_CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/tmuxai"
TARGET_CONFIG_FILE="${TARGET_CONFIG_DIR}/config.yaml"

TMUX_CONF_FILE="${HOME}/.tmux.conf"

TMP_DIR=""

err(){ echo "[ERROR] $*" >&2; exit 1; }
info(){ echo "$*"; }

command_exists(){ command -v "$1" >/dev/null 2>&1; }

tmux_install_hint() {
  if command_exists apt-get; then
    info "  Debian/Ubuntu: sudo apt-get update && sudo apt-get install -y tmux"
  elif command_exists dnf; then
    info "  RHEL/Fedora: sudo dnf install -y tmux"
  elif command_exists yum; then
    info "  RHEL/CentOS: sudo yum install -y tmux"
  elif command_exists pacman; then
    info "  Arch: sudo pacman -Sy --noconfirm tmux"
  elif command_exists zypper; then
    info "  SUSE: sudo zypper install -y tmux"
  elif command_exists apk; then
    info "  Alpine: sudo apk add tmux"
  elif command_exists brew; then
    info "  macOS (Homebrew): brew install tmux"
  else
    info "  Please install tmux using your system package manager."
  fi
}

install_tmux() {
  local sudo_cmd=""
  if [ "${EUID:-0}" -ne 0 ]; then
    command_exists sudo || return 1
    sudo_cmd="sudo"
  fi

  if command_exists apt-get; then
    $sudo_cmd apt-get update && $sudo_cmd apt-get install -y tmux
  elif command_exists dnf; then
    $sudo_cmd dnf install -y tmux
  elif command_exists yum; then
    $sudo_cmd yum install -y tmux
  elif command_exists pacman; then
    $sudo_cmd pacman -Sy --noconfirm tmux
  elif command_exists zypper; then
    $sudo_cmd zypper install -y tmux
  elif command_exists apk; then
    $sudo_cmd apk add tmux
  elif command_exists brew; then
    brew install tmux
  else
    return 1
  fi
}

ensure_tmux_or_exit() {
  if command_exists tmux; then
    return 0
  fi

  info "tmux not found. Trying to install tmux..."
  if install_tmux && command_exists tmux; then
    info "tmux installed."
    return 0
  fi

  info "[ERROR] Failed to install tmux automatically."
  info "Please install tmux manually, then re-run this script."
  info "How to install tmux:"
  tmux_install_hint
  exit 1
}

cleanup() {
  if [ -n "${TMP_DIR:-}" ] && [ -d "${TMP_DIR:-}" ]; then
    rm -rf "$TMP_DIR"
  fi
}

sha256_file() {
  local f="$1"
  if command_exists sha256sum; then
    sha256sum "$f" | awk '{print $1}'
  elif command_exists shasum; then
    shasum -a 256 "$f" | awk '{print $1}'
  else
    err "Need sha256sum (Linux) or shasum (macOS) to verify checksum."
  fi
}

ensure_tmux_conf_lines() {
  local f="$1"
  touch "$f" || err "Failed to write $f"

  local l1='set -g default-terminal "screen-256color"'
  local l2='set -ga terminal-overrides ",*256col*:Tc"'
  local l3='set -g pane-border-style fg=white'
  local l4='set -g pane-active-border-style fg=white'
  local l5='set -g focus-events on'

  grep -Fqx "$l1" "$f" || echo "$l1" >> "$f"
  grep -Fqx "$l2" "$f" || echo "$l2" >> "$f"
  grep -Fqx "$l3" "$f" || echo "$l3" >> "$f"
  grep -Fqx "$l4" "$f" || echo "$l4" >> "$f"
  grep -Fqx "$l5" "$f" || echo "$l5" >> "$f"
}

main() {
  local version="$DEFAULT_VERSION"
  local base_url="$DEFAULT_BASE_URL"
  local bin_dir="$DEFAULT_BIN_DIR"

  local skip_tmux_conf="false"
  local strict_tls="false"

  while [ $# -gt 0 ]; do
    case "$1" in
      -V|--version) version="$2"; shift 2;;
      --base-url) base_url="$2"; shift 2;;
      -b|--bin-dir) bin_dir="$2"; shift 2;;
      --skip-tmux-conf) skip_tmux_conf="true"; shift;;
      --strict-tls) strict_tls="true"; shift;;
      v*) version="$1"; shift;;
      *) err "Unknown argument: $1";;
    esac
  done

  command_exists curl || err "curl is required"
  command_exists tar || err "tar is required"
  command_exists mktemp || err "mktemp is required"
  command_exists grep || err "grep is required"
  command_exists awk || err "awk is required"
  command_exists uname || err "uname is required"
  command_exists tr || err "tr is required"
  command_exists find || err "find is required"
  command_exists head || err "head is required"

  local -a CURL_OPTS
  CURL_OPTS=(-fsSL)
  if [ "$strict_tls" != "true" ]; then
    CURL_OPTS+=(-k)
  fi

  local os_raw arch_raw os arch
  os_raw="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch_raw="$(uname -m)"

  case "$os_raw" in
    linux) os="linux" ;;
    darwin) os="darwin" ;;
    *) err "Unsupported OS: $(uname -s)" ;;
  esac

  ensure_tmux_or_exit

  case "$arch_raw" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    armv7*|armv7l) arch="armv7" ;;
    armv6*|armv6l) arch="armv6" ;;
    *) err "Unsupported arch: $arch_raw" ;;
  esac

  local resolved_from_latest="false"
  if [ "$version" = "latest" ]; then
    local latest_url="${base_url}/release/latest.txt"
    info "Resolving latest version from: ${latest_url}"
    version="$(curl "${CURL_OPTS[@]}" "$latest_url" | tr -d '\r' | head -n 1 | awk '{print $1}')"
    [ -n "$version" ] || err "latest.txt is empty or unreadable: ${latest_url}"
    resolved_from_latest="true"
  fi

  local archive_name="${ARCHIVE_PREFIX}_${os}_${arch}.tar.gz"
  local checksums_url="${base_url}/release/${version}/checksums.sha256"
  local archive_url="${base_url}/release/${version}/${archive_name}"
  local config_url="${base_url}/release/${version}/config.yaml"

  TMP_DIR="$(mktemp -d -t cnpai_install_XXXXXX)"
  trap cleanup EXIT

  info "${PRODUCT_NAME} installer"
  if [ "$resolved_from_latest" = "true" ]; then
    info "Version: ${version} (latest)"
  else
    info "Version: ${version}"
  fi
  info "Platform: ${os}_${arch}"
  info "Archive: ${archive_url}"
  info "Checksums: ${checksums_url}"
  if [ "$strict_tls" != "true" ]; then
    info "TLS verify: disabled (built-in)"
  fi
  info ""

  info "Downloading checksums..."
  curl "${CURL_OPTS[@]}" "$checksums_url" -o "$TMP_DIR/checksums.sha256" || err "Failed to download checksums.sha256"
  tr -d '\r' < "$TMP_DIR/checksums.sha256" > "$TMP_DIR/checksums.clean"

  local expected_sha
  expected_sha="$(
    awk -v f="$archive_name" '
      NF>=2 {
        fn=$2
        sub(/^.*\//,"",fn)
        if (fn==f) {print $1; exit}
      }
    ' "$TMP_DIR/checksums.clean"
  )"

  if [ -z "$expected_sha" ]; then
    info "Available checksum entries:"
    awk 'NF>=2{fn=$2; sub(/^.*\//,"",fn); print " - " fn}' "$TMP_DIR/checksums.clean" | head -n 50
    err "No checksum entry found for ${archive_name} in checksums.sha256"
  fi

  info "Downloading archive..."
  curl "${CURL_OPTS[@]}" "$archive_url" -o "$TMP_DIR/${archive_name}" || err "Failed to download archive"

  local actual_sha
  actual_sha="$(sha256_file "$TMP_DIR/${archive_name}")"
  [ "$actual_sha" = "$expected_sha" ] || err "Checksum mismatch: expected=$expected_sha actual=$actual_sha"

  info "Downloading config..."
  curl "${CURL_OPTS[@]}" "$config_url" -o "$TMP_DIR/config.yaml" || err "Failed to download config.yaml"

  info "Extracting..."
  mkdir -p "$TMP_DIR/extract"
  tar -xzf "$TMP_DIR/${archive_name}" -C "$TMP_DIR/extract" || err "Failed to extract archive"

  local bin_path="$TMP_DIR/extract/${BIN_NAME_IN_ARCHIVE}"
  if [ ! -f "$bin_path" ]; then
    bin_path="$(find "$TMP_DIR/extract" -maxdepth 3 -type f -name "$BIN_NAME_IN_ARCHIVE" -print -quit || true)"
  fi
  [ -f "$bin_path" ] || err "Binary '${BIN_NAME_IN_ARCHIVE}' not found in archive"

  mkdir -p "$bin_dir" || err "Failed to create bin dir: $bin_dir"

  local sudo_cmd=""
  if [ -d "$bin_dir" ] && [ ! -w "$bin_dir" ]; then
    command_exists sudo || err "Need sudo to write to $bin_dir (or use -b to choose a writable dir)"
    sudo_cmd="sudo"
  fi

  info "Installing binary"
  if command_exists install; then
    $sudo_cmd install -m 755 "$bin_path" "${bin_dir}/${BIN_NAME_IN_ARCHIVE}" || err "Install failed"
  else
    $sudo_cmd cp "$bin_path" "${bin_dir}/${BIN_NAME_IN_ARCHIVE}" || err "Copy failed"
    $sudo_cmd chmod 755 "${bin_dir}/${BIN_NAME_IN_ARCHIVE}" || err "chmod failed"
  fi

  mkdir -p "$TARGET_CONFIG_DIR" || err "Failed to create config dir: $TARGET_CONFIG_DIR"

  info "Installing config"
  cp -f "$TMP_DIR/config.yaml" "$TARGET_CONFIG_FILE" || err "Failed to write config.yaml"

  if [ "$skip_tmux_conf" != "true" ]; then
    info "Updating tmux config"
    ensure_tmux_conf_lines "$TMUX_CONF_FILE"
  fi

  info ""
  info "Done."
}

main "$@"