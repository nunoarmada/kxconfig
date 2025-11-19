# kubectx-config

Bash tools to manage multiple Kubernetes kubeconfigs organized by project.

## 📋 Description

`kubectx-config` is a set of tools that allows you to manage Kubernetes kubeconfigs organized by project, making it easier to work with multiple clusters and environments.

### Components

- **`kxconfig`**: Main tool to merge, rename, and remove contexts from kubeconfigs organized by project
- **`kxswap`**: Auxiliary tool to quickly swap the active kubeconfig to a specific project

## ✨ Features

### kxconfig

- ✅ **Merge** new kubeconfigs into a master per project (`~/.kube/config-<name>`)
- ✅ **Rename contexts** in master files (`~/.kube/config-<name>`)
- ✅ **Remove contexts** from master files (`~/.kube/config-<name>`)
- ✅ **Create new projects** interactively
- ✅ **Automatic backup** before any changes
- ✅ **Validation** of kubeconfigs before processing
- ✅ **"Nothing to do" detection** (avoids unnecessary changes)
- ✅ **Automatic integration** with `kxswap` (if available) to apply changes to active config

### kxswap

- ✅ **Quick swap** of the active kubeconfig (`~/.kube/config`)
- ✅ **Validation** of source file
- ✅ **Automatic configuration** of default namespace

## 📦 Requirements

- `bash` (version 4.0 or higher)
- `kubectl` installed and in `PATH`
- Write permissions in `~/.kube/`

## 🚀 Installation

### Method 1: Manual Installation

```bash
# Clone the repository
git clone https://github.com/nunoarmada/kubectx-config.git
cd kubectx-config

# Copy scripts to a directory in PATH
cp kxconfig kxswap ~/.local/bin/

# Or create symlinks
ln -s $(pwd)/kxconfig ~/.local/bin/kxconfig
ln -s $(pwd)/kxswap ~/.local/bin/kxswap

# Ensure they are executable
chmod +x ~/.local/bin/kxconfig ~/.local/bin/kxswap
```

### Method 2: Automatic Script Installation

The installation script automatically checks:
- ✅ If `kubectl` is installed
- ✅ Shell type (bash, zsh, fish)
- ✅ If `~/.local/bin` is in PATH
- ✅ Automatically adds to PATH if necessary

```bash
# Download and execute
curl -fsSL https://raw.githubusercontent.com/nunoarmada/kubectx-config/main/install.sh | bash
```

**Note**: If your shell is not automatically supported (bash, zsh, fish), you'll need to manually add `~/.local/bin` to your PATH.

## 📖 Usage

### kxconfig

#### Merge a new kubeconfig

```bash
# Interactive merge (choose project)
kxconfig kubeconfig-dev.yaml

# Merge into a specific project
kxconfig -p dev kubeconfig-dev.yaml

# Using the -m flag
kxconfig -p dev -m kubeconfig-dev.yaml
```

#### Rename a context

**Note**: This operation renames contexts in the project master file (`~/.kube/config-<name>`), not in the active `~/.kube/config`.

```bash
# Rename context (choose project interactively)
kxconfig -r old-context-name new-context-name

# Rename context in a specific project
kxconfig -p dev -r old-context-name new-context-name
```

#### Remove a context

**Note**: This operation removes contexts from the project master file (`~/.kube/config-<name>`), not from the active `~/.kube/config`.

```bash
# Remove context (choose project interactively)
kxconfig -d context-to-remove

# Remove context from a specific project
kxconfig -p dev -d context-to-remove
```

#### Combined operations

```bash
# Merge + Rename
kxconfig -p dev -m kubeconfig-new.yaml -r old-name new-name

# Merge + Remove
kxconfig -p dev -m kubeconfig-new.yaml -d unwanted-context

# Rename + Remove
kxconfig -p dev -r old-name new-name -d unwanted-context
```

### kxswap

```bash
# Swap to a specific project
kxswap dev

# This will:
# 1. Copy ~/.kube/config-dev to ~/.kube/config
# 2. Configure the default namespace
# 3. kubectl/kubectx will now use this project
```

## 📁 File Structure

`kxconfig` organizes kubeconfigs as follows:

```
~/.kube/
├── config              # Active kubeconfig (used by kubectl)
├── config-dev          # Master for "dev" project
├── config-prod         # Master for "prod" project
├── config-staging      # Master for "staging" project
└── backups/
    ├── dev.bak         # Backup for "dev" project
    ├── prod.bak        # Backup for "prod" project
    └── staging.bak     # Backup for "staging" project
```

## 🔧 Use Case Examples

### Case 1: Add a new cluster to the development project

```bash
# Download the new cluster's kubeconfig
kubectl --kubeconfig=novo-cluster.yaml config view --flatten > novo-cluster.yaml

# Add to dev project
kxconfig -p dev novo-cluster.yaml
```

### Case 2: Organize contexts with consistent names

```bash
# Rename contexts to follow a convention
kxconfig -p dev -r cluster1-context dev-cluster1
kxconfig -p dev -r cluster2-context dev-cluster2
```

### Case 3: Clean up old contexts

```bash
# Remove contexts that are no longer needed
kxconfig -p dev -d old-cluster-context
```

### Case 4: Work with multiple projects

```bash
# Work on dev project
kxswap dev
kubectl get pods

# Switch to production
kxswap prod
kubectl get pods
```

## ⚠️ Important Notes

1. **Backups**: `kxconfig` automatically creates backups in `~/.kube/backups/<project>.bak` before any changes. These backups are unique (they overwrite the previous one).

2. **Validation**: All kubeconfigs are validated before processing using `kubectl config view`.

3. **Security**: Kubeconfig files contain sensitive credentials. Make sure you have appropriate permissions configured.

4. **Integration with kxswap**: All `kxconfig` operations are performed on master files (`~/.kube/config-<name>`). If `kxswap` is available in PATH, `kxconfig` will automatically try to apply these changes to the active kubeconfig (`~/.kube/config`) after completing operations.

## 🐛 Troubleshooting

### Error: "kubectl not found in PATH"

**Solution**: Install `kubectl` and ensure it's in your PATH.

```bash
# Check if kubectl is installed
which kubectl

# If not, install following the official documentation:
# https://kubernetes.io/docs/tasks/tools/
```

### Error: "Source file does not exist"

**Solution**: Check the file path and ensure the project exists.

```bash
# List available projects
ls ~/.kube/config-*

# Check if the file exists
ls -la ~/.kube/config-dev
```

### Error: "kubeconfig appears invalid"

**Solution**: Validate the kubeconfig manually.

```bash
# Validate kubeconfig
kubectl --kubeconfig=your-kubeconfig.yaml config view

# If it fails, the file may be corrupted or malformed
```

### Contexts don't appear after merge

**Solution**: Verify if contexts were actually added.

```bash
# View contexts in the project
kubectl --kubeconfig=~/.kube/config-dev config get-contexts

# If needed, merge again
```

## 🤝 Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for more details on how to contribute.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Inspired by the needs of managing multiple Kubernetes clusters
- Based on best practices for kubeconfig management

## 📝 Changelog

The changelog is automatically generated from commits following the [Conventional Commits](https://www.conventionalcommits.org/) format.

See [releases](https://github.com/nunoarmada/kubectx-config/releases) for the complete version history.

### Version 1.0.0
- Initial functionality
- Support for merge, rename and delete contexts
- Integration with kxswap
- Automatic backups

---

**Developed with ❤️ for the Kubernetes community**
