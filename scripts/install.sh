#!/usr/bin/env bash
# CLUE CODE — install script (Phase 1 stub)
#
# Detects the local environment and prints the dependency plan.
# Use --dry-run (default) to preview without installing.
# Use --apply to actually run the installs.

set -euo pipefail

DRY_RUN=true
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --apply)   DRY_RUN=false ;;
    -h|--help)
      cat <<'EOF'
Usage: install.sh [--dry-run | --apply]

Default: --dry-run (no installation, only the plan).
EOF
      exit 0
      ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

c_reset=$'\033[0m'; c_bold=$'\033[1m'; c_dim=$'\033[2m'
c_green=$'\033[32m'; c_yellow=$'\033[33m'; c_red=$'\033[31m'; c_cyan=$'\033[36m'

log()  { printf "%b[ clue-code ]%b %s\n" "${c_cyan}" "${c_reset}" "$*"; }
ok()   { printf "%b ✓ %b%s\n"            "${c_green}" "${c_reset}" "$*"; }
warn() { printf "%b ! %b%s\n"            "${c_yellow}" "${c_reset}" "$*"; }
err()  { printf "%b ✗ %b%s\n"            "${c_red}" "${c_reset}" "$*"; }
hr()   { printf "%b%s%b\n" "${c_dim}" "----------------------------------------" "${c_reset}"; }

OS="$(uname -s)"
ARCH="$(uname -m)"
log "Detected: ${c_bold}${OS}/${ARCH}${c_reset}"
hr

# --- Plan dependencies -------------------------------------------------------
declare -a DEPS_PRESENT=()
declare -a DEPS_MISSING=()

check() {
  local bin="$1" purpose="$2"
  if command -v "$bin" >/dev/null 2>&1; then
    DEPS_PRESENT+=("$bin :: $purpose")
  else
    DEPS_MISSING+=("$bin :: $purpose")
  fi
}

check go     "Go toolchain (build clue-code from source)"
check git    "git (repo-aware operations)"
check aider  "Aider — edit engine (Phase 2+)"
check ollama "Ollama — local model runtime (cross-platform fallback)"
check python3 "Python 3 (LoRA pipeline, Phase 5+)"

if [[ "$OS" == "Darwin" ]]; then
  check mlx_lm.generate "MLX-LM — Apple Silicon native inference (preferred on macOS)"
fi

# --- Print state -------------------------------------------------------------
log "Dependencies present:"
if [[ ${#DEPS_PRESENT[@]} -eq 0 ]]; then
  warn "  (none)"
else
  for d in "${DEPS_PRESENT[@]}"; do ok "  $d"; done
fi
hr
log "Dependencies missing:"
if [[ ${#DEPS_MISSING[@]} -eq 0 ]]; then
  ok "  (none — all dependencies are installed)"
else
  for d in "${DEPS_MISSING[@]}"; do warn "  $d"; done
fi
hr

# --- Plan --------------------------------------------------------------------
log "Installation plan:"
if [[ "$OS" == "Darwin" ]]; then
  PKG_MGR="brew"
elif [[ "$OS" == "Linux" ]]; then
  if   command -v apt-get >/dev/null 2>&1; then PKG_MGR="apt"
  elif command -v dnf     >/dev/null 2>&1; then PKG_MGR="dnf"
  elif command -v pacman  >/dev/null 2>&1; then PKG_MGR="pacman"
  else PKG_MGR="(none detected)"
  fi
else
  PKG_MGR="(unsupported OS)"
fi
log "  Package manager:  ${c_bold}${PKG_MGR}${c_reset}"

cat <<EOF

Recommended commands (review before running):
  - Go toolchain:   brew install go              (or your package manager equivalent)
  - Aider:          pipx install aider-chat       (Python 3.10+ required)
  - Ollama:         curl -fsSL https://ollama.com/install.sh | sh
  - MLX-LM (macOS): pip install mlx-lm            (requires Python 3.9+)
  - Models (later): ollama pull qwen2.5-coder:7b
                    mlx_lm.generate --model mlx-community/Qwen2.5-Coder-32B-Instruct-4bit ...

EOF

if $DRY_RUN; then
  log "${c_bold}--dry-run mode${c_reset}: nothing was installed."
  log "Re-run with --apply to execute the plan (interactive prompts will appear)."
  exit 0
fi

err "--apply mode is intentionally not implemented in Phase 1."
err "Install dependencies manually using the recommended commands above."
exit 1
