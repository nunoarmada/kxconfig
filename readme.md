# kubectx-merge

Ferramentas em Bash para gerir múltiplos kubeconfigs do Kubernetes de forma organizada por projeto.

## 📋 Descrição

O `kubectx-merge` é um conjunto de ferramentas que permite gerir kubeconfigs do Kubernetes de forma organizada por projeto, facilitando o trabalho com múltiplos clusters e ambientes.

### Componentes

- **`kxmerge`**: Ferramenta principal para fazer merge, renomear e remover contextos de kubeconfigs organizados por projeto
- **`kxswap`**: Ferramenta auxiliar para trocar rapidamente o kubeconfig ativo para um projeto específico

## ✨ Funcionalidades

### kxmerge

- ✅ **Merge** de novos kubeconfigs num master por projeto (`~/.kube/config-<nome>`)
- ✅ **Renomear contextos** dentro de um kubeconfig master
- ✅ **Remover contextos** de um kubeconfig master
- ✅ **Criar novos projetos** interativamente
- ✅ **Backup automático** antes de qualquer alteração
- ✅ **Validação** de kubeconfigs antes de processar
- ✅ **Deteção de "nada a fazer"** (evita alterações desnecessárias)
- ✅ **Integração automática** com `kxswap` (se disponível)

### kxswap

- ✅ **Troca rápida** do kubeconfig ativo (`~/.kube/config`)
- ✅ **Validação** do ficheiro de origem
- ✅ **Configuração automática** do namespace default

## 📦 Requisitos

- `bash` (versão 4.0 ou superior)
- `kubectl` instalado e no `PATH`
- Permissões de escrita em `~/.kube/`

## 🚀 Instalação

### Método 1: Instalação Manual

```bash
# Clonar o repositório
git clone https://github.com/seu-usuario/kubectx-merge.git
cd kubectx-merge

# Copiar os scripts para um diretório no PATH
cp kxmerge kxswap ~/.local/bin/

# Ou criar symlinks
ln -s $(pwd)/kxmerge ~/.local/bin/kxmerge
ln -s $(pwd)/kxswap ~/.local/bin/kxswap

# Garantir que são executáveis
chmod +x ~/.local/bin/kxmerge ~/.local/bin/kxswap
```

### Método 2: Instalação via Script

```bash
# Fazer download e executar
curl -fsSL https://raw.githubusercontent.com/seu-usuario/kubectx-merge/main/install.sh | bash
```

**Nota**: Garante que `~/.local/bin` (ou outro diretório de tua escolha) está no teu `PATH`.

## 📖 Uso

### kxmerge

#### Merge de um novo kubeconfig

```bash
# Merge interativo (escolhe o projeto)
kxmerge kubeconfig-dev.yaml

# Merge num projeto específico
kxmerge -p dev kubeconfig-dev.yaml

# Usando a flag -m
kxmerge -p dev -m kubeconfig-dev.yaml
```

#### Renomear um contexto

```bash
# Renomear contexto (escolhe projeto interativamente)
kxmerge -r old-context-name new-context-name

# Renomear contexto num projeto específico
kxmerge -p dev -r old-context-name new-context-name
```

#### Remover um contexto

```bash
# Remover contexto (escolhe projeto interativamente)
kxmerge -d context-to-remove

# Remover contexto num projeto específico
kxmerge -p dev -d context-to-remove
```

#### Operações combinadas

```bash
# Merge + Renomear
kxmerge -p dev -m kubeconfig-new.yaml -r old-name new-name

# Merge + Remover
kxmerge -p dev -m kubeconfig-new.yaml -d unwanted-context

# Renomear + Remover
kxmerge -p dev -r old-name new-name -d unwanted-context
```

### kxswap

```bash
# Trocar para um projeto específico
kxswap dev

# Isto irá:
# 1. Copiar ~/.kube/config-dev para ~/.kube/config
# 2. Configurar o namespace default
# 3. kubectl/kubectx passarão a usar este projeto
```

## 📁 Estrutura de Ficheiros

O `kxmerge` organiza os kubeconfigs da seguinte forma:

```
~/.kube/
├── config              # Kubeconfig ativo (usado por kubectl)
├── config-dev          # Master do projeto "dev"
├── config-prod         # Master do projeto "prod"
├── config-staging      # Master do projeto "staging"
└── backups/
    ├── dev.bak         # Backup do projeto "dev"
    ├── prod.bak        # Backup do projeto "prod"
    └── staging.bak     # Backup do projeto "staging"
```

## 🔧 Exemplos de Casos de Uso

### Caso 1: Adicionar um novo cluster ao projeto de desenvolvimento

```bash
# Fazer download do kubeconfig do novo cluster
kubectl --kubeconfig=novo-cluster.yaml config view --flatten > novo-cluster.yaml

# Adicionar ao projeto dev
kxmerge -p dev novo-cluster.yaml
```

### Caso 2: Organizar contextos com nomes consistentes

```bash
# Renomear contextos para seguir uma convenção
kxmerge -p dev -r cluster1-context dev-cluster1
kxmerge -p dev -r cluster2-context dev-cluster2
```

### Caso 3: Limpar contextos antigos

```bash
# Remover contextos que já não são necessários
kxmerge -p dev -d old-cluster-context
```

### Caso 4: Trabalhar com múltiplos projetos

```bash
# Trabalhar no projeto dev
kxswap dev
kubectl get pods

# Trocar para produção
kxswap prod
kubectl get pods
```

## ⚠️ Notas Importantes

1. **Backups**: O `kxmerge` cria automaticamente backups em `~/.kube/backups/<projeto>.bak` antes de qualquer alteração. Estes backups são únicos (sobrescrevem o anterior).

2. **Validação**: Todos os kubeconfigs são validados antes de serem processados usando `kubectl config view`.

3. **Segurança**: Os ficheiros de kubeconfig contêm credenciais sensíveis. Garante que tens as permissões adequadas configuradas.

4. **Integração com kxswap**: Se o `kxswap` estiver disponível no PATH, o `kxmerge` tentará automaticamente aplicar as alterações ao kubeconfig ativo.

## 🐛 Troubleshooting

### Erro: "kubectl não encontrado no PATH"

**Solução**: Instala o `kubectl` e garante que está no teu PATH.

```bash
# Verificar se kubectl está instalado
which kubectl

# Se não estiver, instala seguindo a documentação oficial:
# https://kubernetes.io/docs/tasks/tools/
```

### Erro: "Ficheiro de origem não existe"

**Solução**: Verifica o caminho do ficheiro e garante que o projeto existe.

```bash
# Listar projetos disponíveis
ls ~/.kube/config-*

# Verificar se o ficheiro existe
ls -la ~/.kube/config-dev
```

### Erro: "kubeconfig parece inválido"

**Solução**: Valida o kubeconfig manualmente.

```bash
# Validar kubeconfig
kubectl --kubeconfig=teu-kubeconfig.yaml config view

# Se falhar, o ficheiro pode estar corrompido ou mal formatado
```

### Os contextos não aparecem após o merge

**Solução**: Verifica se os contextos foram realmente adicionados.

```bash
# Ver contextos no projeto
kubectl --kubeconfig=~/.kube/config-dev config get-contexts

# Se necessário, faz merge novamente
```

## 🤝 Contribuir

Contribuições são bem-vindas! Por favor, lê o [CONTRIBUTING.md](CONTRIBUTING.md) para mais detalhes sobre como contribuir.

## 📄 Licença

Este projeto está licenciado sob a Licença MIT - vê o ficheiro [LICENSE](LICENSE) para mais detalhes.

## 🙏 Agradecimentos

- Inspirado nas necessidades de gerir múltiplos clusters Kubernetes
- Baseado nas melhores práticas de gestão de kubeconfigs

## 📝 Changelog

### Versão 1.0.0
- Funcionalidade inicial
- Suporte para merge, rename e delete de contextos
- Integração com kxswap
- Backups automáticos

---

**Desenvolvido com ❤️ para a comunidade Kubernetes**
