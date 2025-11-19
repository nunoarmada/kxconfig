#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

SCRIPT_NAME="$(basename "$0")"
INSTALL_DIR="${HOME}/.local/bin"

# Cores para output
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

# Verificar se kubectl está instalado
check_kubectl() {
  log "A verificar se kubectl está instalado..."
  
  if ! command -v kubectl >/dev/null 2>&1; then
    error "kubectl não encontrado no PATH."
    echo ""
    echo "Por favor, instala o kubectl antes de continuar:"
    echo "  https://kubernetes.io/docs/tasks/tools/"
    echo ""
    echo "Ou usa um gestor de pacotes:"
    echo "  macOS:   brew install kubectl"
    echo "  Linux:   Ver documentação oficial"
    exit 1
  fi
  
  local kubectl_version
  kubectl_version=$(kubectl version --client --short 2>/dev/null | cut -d' ' -f3 || echo "desconhecida")
  success "kubectl encontrado (versão: ${kubectl_version})"
}

# Detetar shell e configurar PATH
detect_shell() {
  local shell_name
  shell_name=$(basename "${SHELL:-/bin/sh}")
  
  log "Shell detetado: ${shell_name}"
  
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
      warning "Shell '${shell_name}' não suportado automaticamente."
      warning "Terás de adicionar manualmente ${INSTALL_DIR} ao teu PATH."
      SHELL_RC=""
      return
      ;;
  esac
  
  success "Ficheiro de configuração: ${SHELL_RC}"
}

# Verificar se o diretório de instalação está no PATH
check_path() {
  log "A verificar se ${INSTALL_DIR} está no PATH..."
  
  if echo "${PATH}" | grep -q "${INSTALL_DIR}"; then
    success "${INSTALL_DIR} já está no PATH"
    return 0
  fi
  
  warning "${INSTALL_DIR} não está no PATH"
  return 1
}

# Adicionar ao PATH se necessário
add_to_path() {
  if [ -z "${SHELL_RC:-}" ]; then
    warning "Não foi possível adicionar automaticamente ao PATH."
    echo ""
    echo "Adiciona manualmente ao teu ficheiro de configuração do shell:"
    echo "  export PATH=\"\${HOME}/.local/bin:\${PATH}\""
    return 1
  fi
  
  local path_line="export PATH=\"\${HOME}/.local/bin:\${PATH}\""
  
  # Verificar se já existe
  if grep -q "\.local/bin" "${SHELL_RC}" 2>/dev/null; then
    success "PATH já configurado em ${SHELL_RC}"
    return 0
  fi
  
  log "A adicionar ${INSTALL_DIR} ao PATH em ${SHELL_RC}..."
  
  # Adicionar comentário e linha
  {
    echo ""
    echo "# kubectx-merge - adicionado por install.sh"
    echo "${path_line}"
  } >> "${SHELL_RC}"
  
  success "PATH atualizado em ${SHELL_RC}"
  warning "Executa 'source ${SHELL_RC}' ou reinicia o terminal para aplicar as alterações."
}

# Criar diretório de instalação
create_install_dir() {
  log "A criar diretório de instalação: ${INSTALL_DIR}"
  
  if [ ! -d "${INSTALL_DIR}" ]; then
    mkdir -p "${INSTALL_DIR}"
    success "Diretório criado: ${INSTALL_DIR}"
  else
    success "Diretório já existe: ${INSTALL_DIR}"
  fi
}

# Instalar ficheiros
install_files() {
  local script_dir
  script_dir="$(cd "$(dirname "$0")" && pwd)"
  
  log "A instalar ficheiros de ${script_dir} para ${INSTALL_DIR}..."
  
  local files=("kxconfig" "kxswap")
  local installed=0
  
  for file in "${files[@]}"; do
    local source_file="${script_dir}/${file}"
    local dest_file="${INSTALL_DIR}/${file}"
    
    if [ ! -f "${source_file}" ]; then
      error "Ficheiro não encontrado: ${source_file}"
      continue
    fi
    
    # Copiar ficheiro
    cp "${source_file}" "${dest_file}"
    chmod +x "${dest_file}"
    
    success "Instalado: ${file}"
    installed=$((installed + 1))
  done
  
  if [ ${installed} -eq 0 ]; then
    error "Nenhum ficheiro foi instalado."
    exit 1
  fi
  
  success "Todos os ficheiros instalados com sucesso"
}

# Verificar instalação
verify_installation() {
  log "A verificar instalação..."
  
  local files=("kxconfig" "kxswap")
  local all_ok=true
  
  for file in "${files[@]}"; do
    local file_path="${INSTALL_DIR}/${file}"
    
    if [ -f "${file_path}" ] && [ -x "${file_path}" ]; then
      success "${file} está instalado e executável"
    else
      error "${file} não está instalado corretamente"
      all_ok=false
    fi
  done
  
  if [ "${all_ok}" = true ]; then
    echo ""
    success "Instalação concluída com sucesso!"
    echo ""
    echo "Para usar as ferramentas:"
    echo "  kxconfig --help"
    echo "  kxswap dev"
    echo ""
    
    if ! check_path; then
      echo "Nota: Podes precisar de reiniciar o terminal ou executar:"
      echo "  source ${SHELL_RC:-~/.bashrc}"
    fi
  else
    error "Instalação incompleta. Verifica os erros acima."
    exit 1
  fi
}

# Função principal
main() {
  echo ""
  log "=== Instalação do kubectx-merge ==="
  echo ""
  
  # Verificações
  check_kubectl
  detect_shell
  
  # Instalação
  create_install_dir
  install_files
  
  # Configuração do PATH
  if ! check_path; then
    add_to_path
  fi
  
  # Verificação final
  verify_installation
}

# Executar
main "$@"

