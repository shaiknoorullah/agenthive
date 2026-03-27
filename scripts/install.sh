#!/usr/bin/env bash
# Auto-download agenthive binary from GitHub Releases.
# Used by TPM plugin and manual installs.

set -euo pipefail

REPO="shaiknoorullah/agenthive"
INSTALL_DIR="${AGENTHIVE_INSTALL_DIR:-${TMUX_PLUGIN_DIR:-$HOME/.local/bin}}"

detect_platform() {
    local os arch

    case "$(uname -s)" in
        Linux*)  os="linux" ;;
        Darwin*) os="darwin" ;;
        *)       echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)             echo "Unsupported arch: $(uname -m)" >&2; exit 1 ;;
    esac

    echo "${os}_${arch}"
}

get_latest_version() {
    local version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' \
        | head -1 \
        | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$version" ]; then
        echo "Failed to fetch latest version" >&2
        exit 1
    fi

    echo "$version"
}

main() {
    local platform version archive_name url tmp_dir

    platform=$(detect_platform)
    version="${1:-$(get_latest_version)}"
    archive_name="agenthive_${version#v}_${platform}.tar.gz"
    url="https://github.com/${REPO}/releases/download/${version}/${archive_name}"

    echo "Installing agenthive ${version} for ${platform}..."

    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    curl -fsSL "$url" -o "${tmp_dir}/${archive_name}"
    tar -xzf "${tmp_dir}/${archive_name}" -C "$tmp_dir"

    mkdir -p "$INSTALL_DIR"
    mv "${tmp_dir}/agenthive" "${INSTALL_DIR}/agenthive"
    chmod +x "${INSTALL_DIR}/agenthive"

    echo "Installed agenthive ${version} to ${INSTALL_DIR}/agenthive"
}

main "$@"
