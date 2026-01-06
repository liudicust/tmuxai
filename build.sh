#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

BIN_NAME="cnp-ai"
ARCHIVE_PREFIX="cnpai"

OUT_DIR="$ROOT_DIR/dist"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

sha256_file() {
  local f="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$f" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$f" | awk '{print $1}'
  else
    echo "[ERROR] Need sha256sum (Linux) or shasum (macOS)" >&2
    exit 1
  fi
}

build_one() {
  local os="$1"
  local arch="$2"
  local archive_name="${ARCHIVE_PREFIX}_${os}_${arch}.tar.gz"

  local stage_dir="$OUT_DIR/stage_${os}_${arch}"
  rm -rf "$stage_dir"
  mkdir -p "$stage_dir"

  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w" -o "$stage_dir/$BIN_NAME" main.go

  tar -C "$stage_dir" -czf "$OUT_DIR/$archive_name" "$BIN_NAME"

  rm -rf "$stage_dir"
}

build_one linux amd64
build_one darwin arm64
build_one darwin amd64

: > "$OUT_DIR/checksums.sha256"
(
  cd "$OUT_DIR"
  for f in ${ARCHIVE_PREFIX}_*.tar.gz; do
    [ -f "$f" ] || continue
    printf "%s  %s\n" "$(sha256_file "$f")" "$f"
  done
) >> "$OUT_DIR/checksums.sha256"

echo "Artifacts:" 
ls -1 "$OUT_DIR" | sed 's/^/ - /'
