# Guia de Contribuição

Obrigado por considerares contribuir para o `kubectx-merge`! Este documento fornece diretrizes para contribuições.

## 📋 Como Contribuir

### Reportar Bugs

Se encontrares um bug, por favor:

1. Verifica se o bug já não foi reportado nas [Issues](https://github.com/seu-usuario/kubectx-merge/issues)
2. Cria uma nova issue com:
   - Descrição clara do problema
   - Passos para reproduzir
   - Comportamento esperado vs. comportamento atual
   - Versão do bash e kubectl
   - Sistema operativo

### Sugerir Funcionalidades

Temos todo o gosto em ouvir as tuas ideias! Para sugerir uma nova funcionalidade:

1. Verifica se já não existe uma issue similar
2. Cria uma nova issue com:
   - Descrição clara da funcionalidade
   - Casos de uso e exemplos
   - Benefícios para os utilizadores

### Contribuir com Código

#### Setup do Ambiente

1. Faz fork do repositório
2. Clona o teu fork:
   ```bash
   git clone https://github.com/teu-usuario/kubectx-merge.git
   cd kubectx-merge
   ```

#### Processo de Desenvolvimento

1. Cria uma branch para a tua feature/correção:
   ```bash
   git checkout -b feature/minha-feature
   # ou
   git checkout -b fix/correcao-bug
   ```

2. Faz as alterações necessárias

3. Testa as alterações:
   ```bash
   # Testa os scripts manualmente
   ./kxconfig --help
   ./kxswap dev
   ```

4. Garante que o código segue as convenções:
   - Usa `set -euo pipefail` nos scripts bash
   - Mantém mensagens de erro claras e informativas
   - Adiciona comentários quando necessário
   - Segue o estilo de código existente

5. Faz commit das alterações:
   ```bash
   git add .
   git commit -m "Descrição clara da alteração"
   ```

6. Faz push para o teu fork:
   ```bash
   git push origin feature/minha-feature
   ```

7. Abre um Pull Request no repositório original

#### Convenções de Código

- **Bash**: Usa `set -euo pipefail` e `IFS=$'\n\t'`
- **Nomes de variáveis**: UPPERCASE para variáveis globais
- **Funções**: Nomes descritivos em lowercase com hífens
- **Mensagens**: Em português (PT-PT) para mensagens user-facing
- **Comentários**: Em português, claros e concisos

#### Convenções de Commits

Este projeto usa [Conventional Commits](https://www.conventionalcommits.org/) para automatizar releases. Por favor, usa o seguinte formato:

```
<tipo>(<âmbito>): <descrição>

[corpo opcional]

[rodapé opcional]
```

**Tipos:**
- `feat`: Nova funcionalidade
- `fix`: Correção de bug
- `docs`: Alterações na documentação
- `style`: Formatação, ponto e vírgula, etc. (não afeta código)
- `refactor`: Refatoração de código
- `perf`: Melhorias de performance
- `test`: Adição ou correção de testes
- `chore`: Tarefas de manutenção (dependências, build, etc.)

**Exemplos:**
```
feat(kxconfig): adiciona suporte para listar projetos
fix(kxswap): corrige validação de kubeconfig inválido
docs(readme): atualiza instruções de instalação
```

**Nota**: Commits que seguem o formato conventional commits geram automaticamente releases e changelogs.

#### Exemplo de Estrutura de Script

```bash
#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

# Variáveis globais
SCRIPT_NAME="$(basename "$0")"
BASE_DIR="${HOME}/.kube"

# Funções auxiliares
log() {
  echo "[$SCRIPT_NAME] $*"
}

err() {
  echo "[$SCRIPT_NAME] ERRO: $*" >&2
}

# Função principal
main() {
  # código aqui
}

main "$@"
```

### Revisão de Pull Requests

- Todos os PRs serão revistos
- Pode ser pedido para fazer alterações antes de fazer merge
- Mantém os PRs focados numa única alteração quando possível

## 📝 Checklist para Pull Requests

Antes de submeteres um PR, garante que:

- [ ] O código funciona corretamente
- [ ] Testaste as alterações manualmente
- [ ] Segues as convenções de código
- [ ] Adicionaste comentários quando necessário
- [ ] Atualizaste a documentação se necessário
- [ ] As mensagens de commit são claras e descritivas

## 🎯 Áreas onde Precisamos de Ajuda

- Testes automatizados
- Melhorias na documentação
- Suporte para mais sistemas operativos
- Funcionalidades adicionais (ver issues)

## 📞 Contacto

Se tiveres dúvidas, podes:
- Abrir uma issue
- Criar uma discussão no GitHub

Obrigado por contribuíres! 🎉

