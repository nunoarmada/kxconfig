#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

SCRIPT_NAME="$(basename "$0")"
INSTALL_DIR="${HOME}/.local/bin"
COMPLETION_DIR="${HOME}/.local/share/kxconfig"

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

# Check if kubectl is installed
check_kubectl() {
  log "Checking if kubectl is installed..."
  
  if ! command -v kubectl >/dev/null 2>&1; then
    error "kubectl not found in PATH."
    echo ""
    echo "Please install kubectl before continuing:"
    echo "  https://kubernetes.io/docs/tasks/tools/"
    echo ""
    echo "Or use a package manager:"
    echo "  macOS:   brew install kubectl"
    echo "  Linux:   See official documentation"
    exit 1
  fi
  
  local kubectl_version
  kubectl_version=$(kubectl version --client --short 2>/dev/null | cut -d' ' -f3 || echo "unknown")
  success "kubectl found (version: ${kubectl_version})"
}

# Detect shell and configure PATH
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
      warning "You'll need to manually add ${INSTALL_DIR} to your PATH."
      SHELL_RC=""
      return
      ;;
  esac
  
  success "Configuration file: ${SHELL_RC}"
}

# Check if installation directory is in PATH
check_path() {
  log "Checking if ${INSTALL_DIR} is in PATH..."
  
  if echo "${PATH}" | grep -q "${INSTALL_DIR}"; then
    success "${INSTALL_DIR} is already in PATH"
    return 0
  fi
  
  warning "${INSTALL_DIR} is not in PATH"
  return 1
}

# Add to PATH if necessary
add_to_path() {
  if [ -z "${SHELL_RC:-}" ]; then
    warning "Could not automatically add to PATH."
    echo ""
    echo "Manually add to your shell configuration file:"
    echo "  export PATH=\"\${HOME}/.local/bin:\${PATH}\""
    return 1
  fi
  
  local path_line="export PATH=\"\${HOME}/.local/bin:\${PATH}\""
  
  # Check if it already exists
  if grep -q "\.local/bin" "${SHELL_RC}" 2>/dev/null; then
    success "PATH already configured in ${SHELL_RC}"
    return 0
  fi
  
  log "Adding ${INSTALL_DIR} to PATH in ${SHELL_RC}..."
  
  # Add comment and line
  {
    echo ""
    echo "# kxconfig - added by install.sh"
    echo "${path_line}"
  } >> "${SHELL_RC}"
  
  success "PATH updated in ${SHELL_RC}"
  warning "Run 'source ${SHELL_RC}' or restart the terminal to apply changes."
}

# Create installation directory
create_install_dir() {
  log "Creating installation directory: ${INSTALL_DIR}"
  
  if [ ! -d "${INSTALL_DIR}" ]; then
    mkdir -p "${INSTALL_DIR}"
    success "Directory created: ${INSTALL_DIR}"
  else
    success "Directory already exists: ${INSTALL_DIR}"
  fi
}

# Install files
install_files() {
  local script_dir
  script_dir="$(cd "$(dirname "$0")" && pwd)"
  local project_root
  project_root="$(cd "${script_dir}/.." && pwd)"
  
  log "Installing files from ${project_root}/bin to ${INSTALL_DIR}..."
  
  local files=("kxconfig" "kxswap")
  local installed=0
  
  for file in "${files[@]}"; do
    local source_file="${project_root}/bin/${file}"
    local dest_file="${INSTALL_DIR}/${file}"
    
    if [ ! -f "${source_file}" ]; then
      error "File not found: ${source_file}"
      continue
    fi
    
    # Copy file
    cp "${source_file}" "${dest_file}"
    chmod +x "${dest_file}"
    
    success "Installed: ${file}"
    installed=$((installed + 1))
  done
  
  if [ ${installed} -eq 0 ]; then
    error "No files were installed."
    exit 1
  fi
  
  success "All files installed successfully"
}

# Install completion script
install_completion() {
  local script_dir
  script_dir="$(cd "$(dirname "$0")" && pwd)"
  local project_root
  project_root="$(cd "${script_dir}/.." && pwd)"
  
  local completion_file="${project_root}/completions/kxconfig-completion.bash"
  
  if [ ! -f "${completion_file}" ]; then
    warning "Completion file not found: ${completion_file}"
    return 1
  fi
  
  log "Installing completion script..."
  
  # Create completion directory
  mkdir -p "${COMPLETION_DIR}"
  
  # Copy completion file
  cp "${completion_file}" "${COMPLETION_DIR}/completion.bash"
  success "Completion script installed to ${COMPLETION_DIR}/completion.bash"
  
  # Add to shell RC if not already present
  if [ -z "${SHELL_RC:-}" ]; then
    warning "Could not automatically add completion to shell configuration."
    echo ""
    echo "Manually add to your shell configuration file:"
    echo "  source ${COMPLETION_DIR}/completion.bash"
    return 1
  fi
  
  local completion_line="source ${COMPLETION_DIR}/completion.bash"
  
  # Check if it already exists
  if grep -q "kxconfig.*completion" "${SHELL_RC}" 2>/dev/null; then
    success "Completion already configured in ${SHELL_RC}"
    return 0
  fi
  
  log "Adding completion to ${SHELL_RC}..."
  
  # Add comment and line
  {
    echo ""
    echo "# kxconfig completion - added by install.sh"
    echo "${completion_line}"
  } >> "${SHELL_RC}"
  
  success "Completion added to ${SHELL_RC}"
  warning "Run 'source ${SHELL_RC}' or restart the terminal to enable completion."
}

# Verify installation
verify_installation() {
  log "Verifying installation..."
  
  local files=("kxconfig" "kxswap")
  local all_ok=true
  
  for file in "${files[@]}"; do
    local file_path="${INSTALL_DIR}/${file}"
    
    if [ -f "${file_path}" ] && [ -x "${file_path}" ]; then
      success "${file} is installed and executable"
    else
      error "${file} is not installed correctly"
      all_ok=false
    fi
  done
  
  if [ "${all_ok}" = true ]; then
    echo ""
    success "Installation completed successfully!"
    echo ""
    echo "To use the tools:"
    echo "  kxconfig --help"
    echo "  kxswap dev"
    echo ""
    
    if ! check_path; then
      echo "Note: You may need to restart the terminal or run:"
      echo "  source ${SHELL_RC:-~/.bashrc}"
    fi
  else
    error "Installation incomplete. Check the errors above."
    exit 1
  fi
}

# Main function
main() {
  echo ""
  log "=== kxconfig Installation ==="
  echo ""
  
  # Checks
  check_kubectl
  detect_shell
  
  # Installation
  create_install_dir
  install_files
  install_completion
  
  # PATH configuration
  if ! check_path; then
    add_to_path
  fi
  
  # Final verification
  verify_installation
}

# Execute
main "$@"

