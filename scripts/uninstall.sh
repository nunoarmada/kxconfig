#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

SCRIPT_NAME="$(basename "$0")"
INSTALL_DIR="${HOME}/.local/bin"
COMPLETION_DIR="${HOME}/.local/share/kubectx-config"
BASE_DIR="${HOME}/.kube"
BACKUP_DIR="${BASE_DIR}/backups"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
  echo -e "${BLUE}[${SCRIPT_NAME}]${NC} $*"
}

success() {
  echo -e "${GREEN}✓${NC} $*"
}

warning() {
  echo -e "${YELLOW}⚠${NC} $*"
}

error() {
  echo -e "${RED}✗${NC} $*" >&2
}

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Description:
  Uninstalls kubectx-config tools and optionally removes all data.

Options:
  -h, --help
      Show this help and exit.

  -a, --all
      Remove everything including backups and project configs.
      WARNING: This will delete all your kubeconfig projects and backups!

  --keep-data (default)
      Keep backups and project configs (config-<name> files).
      Only removes installed scripts and shell configuration.

Examples:
  # Uninstall but keep data (default)
  $SCRIPT_NAME

  # Uninstall and remove everything
  $SCRIPT_NAME --all
EOF
}

# Detect shell and find configuration file
detect_shell() {
  local shell_name
  shell_name=$(basename "${SHELL:-/bin/sh}")
  
  log "Shell detected: ${shell_name}"
  
  case "${shell_name}" in
    bash)
      SHELL_RC="${HOME}/.bashrc"
      [ -f "${HOME}/.bash_profile" ] && SHELL_RC="${HOME}/.bash_profile"
      ;;
    zsh)
      SHELL_RC="${HOME}/.zshrc"
      ;;
    fish)
      SHELL_RC="${HOME}/.config/fish/config.fish"
      ;;
    *)
      warning "Shell '${shell_name}' not automatically supported."
      warning "You'll need to manually remove configuration from your shell config file."
      SHELL_RC=""
      return
      ;;
  esac
  
  success "Configuration file: ${SHELL_RC}"
}

# Remove installed scripts
remove_scripts() {
  log "Removing installed scripts..."
  
  local files=("kxconfig" "kxswap")
  local removed=0
  
  for file in "${files[@]}"; do
    local file_path="${INSTALL_DIR}/${file}"
    
    if [ -f "${file_path}" ]; then
      rm -f "${file_path}"
      success "Removed: ${file}"
      removed=$((removed + 1))
    else
      warning "${file} not found at ${file_path}"
    fi
  done
  
  if [ ${removed} -eq 0 ]; then
    warning "No scripts were found to remove."
  else
    success "Scripts removed successfully"
  fi
}

# Remove completion script and directory
remove_completion() {
  log "Removing completion script..."
  
  if [ -f "${COMPLETION_DIR}/completion.bash" ]; then
    rm -f "${COMPLETION_DIR}/completion.bash"
    success "Removed completion script"
  else
    warning "Completion script not found"
  fi
  
  # Remove directory if empty
  if [ -d "${COMPLETION_DIR}" ]; then
    if [ -z "$(ls -A "${COMPLETION_DIR}")" ]; then
      if rmdir "${COMPLETION_DIR}" 2>/dev/null; then
        success "Removed completion directory"
      fi
    fi
  fi
}

# Remove configuration from shell RC
remove_shell_config() {
  if [ -z "${SHELL_RC:-}" ]; then
    warning "Could not automatically remove shell configuration."
    echo ""
    echo "Manually remove from your shell configuration file:"
    echo "  - Lines containing 'kubectx-config'"
    echo "  - Lines containing '${COMPLETION_DIR}/completion.bash'"
    echo "  - Lines containing '${INSTALL_DIR}' in PATH (if added by install.sh)"
    return 1
  fi
  
  if [ ! -f "${SHELL_RC}" ]; then
    warning "Shell configuration file not found: ${SHELL_RC}"
    return 1
  fi
  
  log "Removing configuration from ${SHELL_RC}..."
  
  # Create a temporary file
  local temp_file
  temp_file=$(mktemp)
  
  # Remove lines containing kubectx-config
  if grep -q "kubectx-config" "${SHELL_RC}" 2>/dev/null; then
    grep -v "kubectx-config" "${SHELL_RC}" > "${temp_file}" || true
    mv "${temp_file}" "${SHELL_RC}"
    success "Removed kubectx-config configuration from ${SHELL_RC}"
  else
    warning "No kubectx-config configuration found in ${SHELL_RC}"
    rm -f "${temp_file}"
  fi
}

# Remove PATH entry (if it was added by install.sh)
remove_path_entry() {
  if [ -z "${SHELL_RC:-}" ]; then
    return 1
  fi
  
  if [ ! -f "${SHELL_RC}" ]; then
    return 1
  fi
  
  # Check if PATH line was added by install.sh
  if grep -q "# kubectx-config.*added by install.sh" "${SHELL_RC}" 2>/dev/null; then
    log "Removing PATH entry from ${SHELL_RC}..."
    
    local temp_file
    temp_file=$(mktemp)
    
    # Remove the PATH line and the comment above it
    awk '
      /# kubectx-config.*added by install.sh/ { skip=1; next }
      skip && /export PATH.*\.local\/bin/ { skip=0; next }
      { print }
    ' "${SHELL_RC}" > "${temp_file}" || true
    
    mv "${temp_file}" "${SHELL_RC}"
    success "Removed PATH entry from ${SHELL_RC}"
  fi
}

# Remove all data (backups and project configs)
remove_all_data() {
  log "Removing all data (backups and project configs)..."
  
  local removed_backups=0
  local removed_configs=0
  
  # Remove backups
  if [ -d "${BACKUP_DIR}" ]; then
    for backup in "${BACKUP_DIR}"/*.bak; do
      if [ -f "$backup" ]; then
        rm -f "$backup"
        removed_backups=$((removed_backups + 1))
      fi
    done
    
    if [ ${removed_backups} -gt 0 ]; then
      success "Removed ${removed_backups} backup file(s)"
    fi
    
    # Remove backup directory if empty
    if [ -z "$(ls -A "${BACKUP_DIR}")" ]; then
      if rmdir "${BACKUP_DIR}" 2>/dev/null; then
        success "Removed backup directory"
      fi
    fi
  fi
  
  # Remove project configs
  if [ -d "${BASE_DIR}" ]; then
    for config in "${BASE_DIR}"/config-*; do
      if [ -f "$config" ]; then
        rm -f "$config"
        removed_configs=$((removed_configs + 1))
      fi
    done
    
    if [ ${removed_configs} -gt 0 ]; then
      success "Removed ${removed_configs} project config file(s)"
    fi
  fi
  
  if [ ${removed_backups} -eq 0 ] && [ ${removed_configs} -eq 0 ]; then
    warning "No data files found to remove"
  fi
}

# Main function
main() {
  local remove_all=false
  
  # Parse arguments
  while [ $# -gt 0 ]; do
    case "$1" in
      -h|--help)
        usage
        exit 0
        ;;
      -a|--all)
        remove_all=true
        shift
        ;;
      --keep-data)
        remove_all=false
        shift
        ;;
      *)
        error "Unknown option: $1"
        usage
        exit 1
        ;;
    esac
  done
  
  echo ""
  log "=== kubectx-config Uninstallation ==="
  echo ""
  
  if [ "$remove_all" = true ]; then
    warning "WARNING: This will remove ALL data including backups and project configs!"
    echo ""
    read -rp "Are you sure you want to continue? [y/N]: " confirm
    confirm="${confirm:-N}"
    case "$confirm" in
      y|Y)
        log "Proceeding with full removal..."
        ;;
      *)
        log "Uninstallation cancelled."
        exit 0
        ;;
    esac
  else
    log "Uninstalling scripts and configuration (keeping data)..."
  fi
  
  echo ""
  
  # Detect shell
  detect_shell
  
  # Remove scripts
  remove_scripts
  
  # Remove completion
  remove_completion
  
  # Remove shell configuration
  remove_shell_config
  
  # Remove PATH entry
  remove_path_entry
  
  # Remove data if requested
  if [ "$remove_all" = true ]; then
    echo ""
    remove_all_data
  else
    echo ""
    success "Data preserved:"
    if [ -d "${BACKUP_DIR}" ]; then
      local backup_count
      backup_count=$(find "${BACKUP_DIR}" -name "*.bak" 2>/dev/null | wc -l | tr -d ' ')
      echo "  - Backups: ${backup_count} file(s) in ${BACKUP_DIR}"
    fi
    if [ -d "${BASE_DIR}" ]; then
      local config_count
      config_count=$(find "${BASE_DIR}" -name "config-*" -type f 2>/dev/null | wc -l | tr -d ' ')
      echo "  - Project configs: ${config_count} file(s) in ${BASE_DIR}"
    fi
    echo ""
    echo "To remove data as well, run: $SCRIPT_NAME --all"
  fi
  
  echo ""
  success "Uninstallation completed!"
  echo ""
  echo "Note: You may need to restart your terminal or run:"
  echo "  source ${SHELL_RC:-~/.bashrc}"
}

# Execute
main "$@"

