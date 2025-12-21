#!/usr/bin/env bash
set -euo pipefail

# Bootstrap installer for the AWK CLI (awkit).
#
# Usage:
#   curl -fsSL https://github.com/<owner>/<repo>/releases/latest/download/install.sh | bash
#   curl -fsSL https://github.com/<owner>/<repo>/releases/latest/download/install.sh | bash -s -- /path/to/project
#
# Optional env vars:
#   AWKIT_REPO    default: silver2dream/ai-workflow-kit
#   AWKIT_VERSION default: latest
#   AWKIT_PREFIX  default: ~/.local

REPO="${AWKIT_REPO:-silver2dream/ai-workflow-kit}"
VERSION="${AWKIT_VERSION:-latest}"
PREFIX="${AWKIT_PREFIX:-$HOME/.local}"

usage() {
  cat <<'EOF'
Usage:
  install.sh [project_path]

Installs `awkit` into ~/.local/bin (by default). If project_path is provided,
it will also run: awkit install <project_path> --preset react-go

Env vars:
  AWKIT_REPO    GitHub repo (owner/name)
  AWKIT_VERSION Release version tag (or 'latest')
  AWKIT_PREFIX  Install prefix (default: ~/.local)
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "ERROR: missing dependency: $1" >&2; exit 1; }
}

need curl
need tar

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  linux|darwin) ;;
  *) echo "ERROR: unsupported OS: $os" >&2; exit 1 ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "ERROR: unsupported arch: $arch" >&2; exit 1 ;;
esac

asset="awkit_${os}_${arch}.tar.gz"
if [[ "$VERSION" == "latest" ]]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
fi

tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

echo "[install] Downloading ${url}"
curl -fsSL "$url" -o "${tmp}/${asset}"

tar -xzf "${tmp}/${asset}" -C "$tmp"

bin_dir="${PREFIX}/bin"
mkdir -p "$bin_dir"
install -m 0755 "${tmp}/awkit" "${bin_dir}/awkit"

echo ""
echo "✓ awkit installed to ${bin_dir}/awkit"

# Check if bin_dir is already in PATH
if [[ ":$PATH:" == *":${bin_dir}:"* ]]; then
  echo "✓ ${bin_dir} is already in PATH"
  echo ""
  echo "Run 'awkit version' to verify installation."
else
  echo ""
  echo "To use awkit, add it to your PATH:"
  echo ""
  
  # Detect shell and give specific advice
  shell_name="$(basename "${SHELL:-/bin/bash}")"
  case "$shell_name" in
    zsh)
      echo "  echo 'export PATH=\"${bin_dir}:\$PATH\"' >> ~/.zshrc"
      echo "  source ~/.zshrc"
      ;;
    bash)
      if [[ -f "$HOME/.bashrc" ]]; then
        echo "  echo 'export PATH=\"${bin_dir}:\$PATH\"' >> ~/.bashrc"
        echo "  source ~/.bashrc"
      else
        echo "  echo 'export PATH=\"${bin_dir}:\$PATH\"' >> ~/.bash_profile"
        echo "  source ~/.bash_profile"
      fi
      ;;
    fish)
      echo "  fish_add_path ${bin_dir}"
      ;;
    *)
      echo "  export PATH=\"${bin_dir}:\$PATH\""
      ;;
  esac
  echo ""
  echo "Or restart your terminal, then run 'awkit version' to verify."
fi
echo ""

if [[ -n "${1:-}" ]]; then
  "${bin_dir}/awkit" install "$1" --preset react-go
fi

