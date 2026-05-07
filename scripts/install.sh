#!/usr/bin/env bash
# CLUE CODE — binary installer
#
# Downloads the pre-built binary from GitHub Releases, verifies the SHA256
# checksum, and installs to /usr/local/bin (or ~/.local/bin if non-root).
#
# Usage:
#   curl -sSL https://github.com/clue-code/clue-code/releases/latest/download/install.sh | sh
#   bash install.sh [--dry-run] [--version v1.0.0] [--install-dir /custom/path]
#
# Flags:
#   --dry-run              Detect OS/arch and print plan; do not download.
#   --version <tag>        Install specific version (default: latest).
#   --install-dir <path>   Override install directory.
#   -h / --help            Show this help.

set -euo pipefail

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
REPO="clue-code/clue-code"
BINARY_NAME="clue-code"
GITHUB_RELEASES="https://github.com/${REPO}/releases"

# ---------------------------------------------------------------------------
# Colours
# ---------------------------------------------------------------------------
c_reset=$'\033[0m'; c_bold=$'\033[1m'; c_dim=$'\033[2m'
c_green=$'\033[32m'; c_yellow=$'\033[33m'; c_red=$'\033[31m'; c_cyan=$'\033[36m'

log()  { printf "%b[ clue-code ]%b %s\n" "${c_cyan}"   "${c_reset}" "$*"; }
ok()   { printf "%b ✓ %b%s\n"            "${c_green}"  "${c_reset}" "$*"; }
warn() { printf "%b ⚠ %b%s\n"            "${c_yellow}" "${c_reset}" "$*"; }
err()  { printf "%b ✗ %b%s\n"            "${c_red}"    "${c_reset}" "$*" >&2; }
hr()   { printf "%b%s%b\n" "${c_dim}" "──────────────────────────────────────────────" "${c_reset}"; }

die() { err "$*"; exit 1; }

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
DRY_RUN=false
VERSION=""
INSTALL_DIR=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)        DRY_RUN=true; shift ;;
    --version)        VERSION="${2:-}"; shift 2 ;;
    --install-dir)    INSTALL_DIR="${2:-}"; shift 2 ;;
    -h|--help)
      cat <<'EOF'
Usage: install.sh [--dry-run] [--version v1.0.0] [--install-dir /path]

  --dry-run        Print detection results only; do not download or install.
  --version <tag>  Install a specific release (e.g. v1.0.0). Default: latest.
  --install-dir    Override installation directory.
  -h, --help       Show this message.
EOF
      exit 0
      ;;
    *) die "Unknown flag: $1" ;;
  esac
done

# ---------------------------------------------------------------------------
# OS / arch detection
# ---------------------------------------------------------------------------
detect_os_arch() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"

  case "$os" in
    Darwin) OS="darwin" ;;
    Linux)  OS="linux"  ;;
    *)      die "Unsupported OS: ${os}. Only macOS and Linux are supported." ;;
  esac

  case "$arch" in
    x86_64|amd64)   ARCH="amd64"  ;;
    arm64|aarch64)  ARCH="arm64"  ;;
    *)              die "Unsupported architecture: ${arch}." ;;
  esac
}

# ---------------------------------------------------------------------------
# Install directory selection
# ---------------------------------------------------------------------------
select_install_dir() {
  if [[ -n "$INSTALL_DIR" ]]; then
    return
  fi
  if [[ -w "/usr/local/bin" ]]; then
    INSTALL_DIR="/usr/local/bin"
  elif [[ "$(id -u)" != "0" ]]; then
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "$INSTALL_DIR"
    # Warn if not in PATH.
    if [[ ":${PATH}:" != *":${INSTALL_DIR}:"* ]]; then
      warn "${INSTALL_DIR} is not in PATH — add it to your shell profile."
    fi
  else
    die "Cannot determine install directory. Use --install-dir."
  fi
}

# ---------------------------------------------------------------------------
# Version resolution
# ---------------------------------------------------------------------------
resolve_version() {
  if [[ -n "$VERSION" ]]; then
    return
  fi
  log "Resolving latest release…"
  # Use gh CLI if available (most reliable); fall back to curl redirect.
  if command -v gh >/dev/null 2>&1; then
    VERSION="$(gh release view --repo "${REPO}" --json tagName -q .tagName 2>/dev/null)" || true
  fi
  if [[ -z "$VERSION" ]]; then
    # Follow GitHub redirect for /releases/latest to extract tag.
    local location
    location="$(curl -sI "${GITHUB_RELEASES}/latest" | grep -i '^location:' | tr -d '\r' | awk '{print $2}')" || true
    VERSION="${location##*/}"
  fi
  if [[ -z "$VERSION" ]]; then
    die "Could not determine latest release. Specify --version explicitly."
  fi
  log "Latest version: ${c_bold}${VERSION}${c_reset}"
}

# ---------------------------------------------------------------------------
# Download helpers
# ---------------------------------------------------------------------------
download_file() {
  local url="$1" dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 3 --retry-delay 1 -o "$dest" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url"
  else
    die "Neither curl nor wget found. Install one and retry."
  fi
}

verify_checksum() {
  local file="$1" expected="$2"
  local actual
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  else
    warn "No SHA256 tool found — skipping checksum verification."
    return 0
  fi
  if [[ "$actual" != "$expected" ]]; then
    die "Checksum mismatch!\n  expected: ${expected}\n  actual:   ${actual}"
  fi
  ok "Checksum verified."
}

# ---------------------------------------------------------------------------
# Optional dependency warnings (non-blocking)
# ---------------------------------------------------------------------------
warn_optional_deps() {
  hr
  log "Optional dependencies:"
  if ! command -v ollama >/dev/null 2>&1; then
    warn "ollama not found — local model runtime unavailable."
    warn "  Install: curl -fsSL https://ollama.com/install.sh | sh"
  else
    ok "ollama found at $(command -v ollama)"
  fi
  if ! command -v aider >/dev/null 2>&1; then
    warn "aider not found — edit engine unavailable."
    warn "  Install: pipx install aider-chat"
  else
    ok "aider found at $(command -v aider)"
  fi
  hr
}

# ---------------------------------------------------------------------------
# Main installation logic
# ---------------------------------------------------------------------------
main() {
  detect_os_arch

  hr
  log "CLUE CODE Installer"
  hr
  log "Detected: ${c_bold}${OS}/${ARCH}${c_reset}"

  select_install_dir
  log "Install dir: ${c_bold}${INSTALL_DIR}${c_reset}"

  resolve_version

  # Archive name follows goreleaser convention: clue-code_{VERSION}_{OS}_{ARCH}.tar.gz
  # Strip leading 'v' for goreleaser artifact naming.
  local ver_stripped="${VERSION#v}"
  local archive="clue-code_${ver_stripped}_${OS}_${ARCH}.tar.gz"
  local checksum_file="clue-code_${ver_stripped}_checksums.txt"
  local base_url="${GITHUB_RELEASES}/download/${VERSION}"

  log "Archive: ${archive}"

  if $DRY_RUN; then
    hr
    log "${c_bold}--dry-run${c_reset}: OS=${OS} ARCH=${ARCH} VERSION=${VERSION}"
    log "Would download: ${base_url}/${archive}"
    log "Would verify:   ${base_url}/${checksum_file}"
    log "Would install:  ${INSTALL_DIR}/${BINARY_NAME}"
    warn_optional_deps
    log "Re-run without --dry-run to install."
    exit 0
  fi

  # --- Download to temp dir ---------------------------------------------------
  local tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  log "Downloading ${archive}…"
  download_file "${base_url}/${archive}" "${tmpdir}/${archive}"

  log "Downloading checksums…"
  download_file "${base_url}/${checksum_file}" "${tmpdir}/${checksum_file}"

  # Extract expected checksum for this archive.
  local expected_hash
  expected_hash="$(grep "${archive}" "${tmpdir}/${checksum_file}" | awk '{print $1}')" || true
  if [[ -z "$expected_hash" ]]; then
    warn "Archive not listed in checksums file — skipping verification."
  else
    verify_checksum "${tmpdir}/${archive}" "$expected_hash"
  fi

  # --- Extract and install ----------------------------------------------------
  tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}"

  local extracted_bin="${tmpdir}/${BINARY_NAME}"
  if [[ ! -f "$extracted_bin" ]]; then
    die "Binary not found in archive. Expected: ${extracted_bin}"
  fi

  chmod +x "$extracted_bin"
  mv "$extracted_bin" "${INSTALL_DIR}/${BINARY_NAME}"

  # macOS Gatekeeper: strip quarantine and provenance extended attributes that
  # cause SIGKILL on first run after download. Silently ignored on Linux.
  if [[ "$OS" == "darwin" ]]; then
    xattr -dr com.apple.quarantine "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null || true
    xattr -dr com.apple.provenance "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null || true
    # Apply ad-hoc codesign so macOS Gatekeeper accepts the binary without
    # requiring an Apple Developer ID. This is sufficient for local builds.
    codesign --force --sign - "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null || true
  fi

  ok "Installed ${c_bold}${INSTALL_DIR}/${BINARY_NAME}${c_reset} (${VERSION})"
  hr

  # --- Post-install doctor (brief) --------------------------------------------
  warn_optional_deps
  log "Running post-install check…"
  if command -v "${INSTALL_DIR}/${BINARY_NAME}" >/dev/null 2>&1 || [[ -x "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
    "${INSTALL_DIR}/${BINARY_NAME}" doctor --brief || true
  fi

  hr
  ok "CLUE CODE ${VERSION} installed successfully!"
  log "Get started: ${c_bold}clue-code chat \"hello\"${c_reset}"
  log "Docs:        https://github.com/${REPO}/tree/main/docs"

  # --- Setup wizard prompt (TTY only) ----------------------------------------
  if [[ -t 0 ]]; then
    hr
    printf "%b💡%b Aucun modele IA n'est encore configure.\n" "${c_yellow}" "${c_reset}"
    printf "   Voulez-vous lancer '%bclue-code setup%b' maintenant ? [O/n] " \
      "${c_bold}" "${c_reset}"
    read -r _setup_answer </dev/tty || _setup_answer="n"
    case "${_setup_answer,,}" in
      ""|o|y|yes|oui)
        "${INSTALL_DIR}/${BINARY_NAME}" setup
        ;;
      *)
        log "Plus tard, lancez: ${c_bold}clue-code setup${c_reset}"
        ;;
    esac
  else
    log "Non-interactive install detected."
    log "Pour configurer un modele IA, lancez: ${c_bold}clue-code setup${c_reset}"
  fi
}

main "$@"
