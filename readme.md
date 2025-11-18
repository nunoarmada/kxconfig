# merge-kubeconfig.sh

Script em Bash para gerir o teu kubeconfig padrão `~/.kube/config`:

- **Merge** de um novo kubeconfig (`-m`)
- **Renomear contexto** (`-r`)
- **Remover contexto** (`-d`)
- Podes fazer **apenas uma operação** ou **várias na mesma execução**

Tudo isto com:

- **Backup automático** antes de qualquer alteração  
- **Validação** do kubeconfig passado em `-m`  
- **Deteção de “nada a fazer”** (se o resultado for igual ao atual)  
- Limpeza de temporários via `trap`

---

## Requisitos

- `bash`
- `kubectl` no `PATH`
- Permissões de escrita em `~/.kube/config` (ou para o criar, em caso de `-m`)

---

## Instalação

```bash
curl -o merge-kubeconfig.sh https://example.com/merge-kubeconfig.sh
chmod +x merge-kubeconfig.sh
# opcional
mv merge-kubeconfig.sh ~/.local/bin/