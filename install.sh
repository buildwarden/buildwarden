#!/bin/sh
set -e

REPO="buildwarden/buildwarden"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       echo "unsupported" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             echo "unsupported" ;;
    esac
}

main() {
    OS=$(detect_os)
    ARCH=$(detect_arch)

    if [ "$OS" = "unsupported" ] || [ "$ARCH" = "unsupported" ]; then
        echo "Error: unsupported platform $(uname -s)/$(uname -m)" >&2
        exit 1
    fi

    if [ -n "$1" ]; then
        VERSION="$1"
    else
        VERSION=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" \
            | grep '"tag_name"' | head -1 | cut -d'"' -f4)
    fi

    if [ -z "$VERSION" ]; then
        echo "Error: could not determine latest version" >&2
        exit 1
    fi

    # Strip leading 'v' for archive name
    VER_NUM="${VERSION#v}"
    ARCHIVE="buildwarden_${VER_NUM}_${OS}_${ARCH}.tar.gz"
    URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

    echo "Installing BuildWarden ${VERSION} (${OS}/${ARCH})..."

    TMP=$(mktemp -d)
    trap 'rm -rf "$TMP"' EXIT

    curl -sSfL -o "${TMP}/${ARCHIVE}" "$URL"
    tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP"

    mkdir -p "$INSTALL_DIR"
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMP}/warden" "${INSTALL_DIR}/warden"
    else
        echo "Installing to ${INSTALL_DIR} (requires sudo)..."
        sudo mv "${TMP}/warden" "${INSTALL_DIR}/warden"
    fi

    echo "Installed: ${INSTALL_DIR}/warden (${VERSION})"

    case ":${PATH}:" in
        *":${INSTALL_DIR}:"*) ;;
        *) echo "Add to your PATH: export PATH=\"${INSTALL_DIR}:\$PATH\"" ;;
    esac
}

main "$@"
