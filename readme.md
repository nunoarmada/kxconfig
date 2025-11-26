# kxconfig

Bash tools to manage multiple Kubernetes kubeconfigs organized by project.

## 📋 Description

`kxconfig` is a set of tools that allows you to manage Kubernetes kubeconfigs organized by project, making it easier to work with multiple clusters and environments.

### Components

- **`kxconfig`**: Main tool to merge, replace, and remove contexts from kubeconfigs organized by project
- **`kxswap`**: Auxiliary tool to quickly swap the active kubeconfig to a specific project

## ✨ Features

### kxconfig

- ✅ **Merge** new kubeconfigs into a master per project (`~/.kube/config-<name>`)
- ✅ **Replace** master kubeconfig (instead of merging)
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

### Method 1: Homebrew (macOS/Linux) - Recommended

```bash
# Install via Homebrew tap
brew install nunoarmada/kxconfig/kxconfig
```

**Note**: This requires a Homebrew tap to be set up. See [docs/HOMEBREW.md](docs/HOMEBREW.md) for setup instructions.

### Method 2: Automatic Script Installation

The installation script automatically checks:
- ✅ If `kubectl` is installed
- ✅ Shell type (bash, zsh, fish)
- ✅ If `~/.local/bin` is in PATH
- ✅ Automatically adds to PATH if necessary

```bash
# Download and execute
curl -fsSL https://raw.githubusercontent.com/nunoarmada/kxconfig/main/scripts/install.sh | bash
```

**Note**: If your shell is not automatically supported (bash, zsh, fish), you'll need to manually add `~/.local/bin` to your PATH.

### Method 3: Manual Installation

```bash
# Clone the repository
git clone https://github.com/nunoarmada/kxconfig.git
cd kxconfig

# Copy scripts to a directory in PATH
cp bin/kxconfig bin/kxswap ~/.local/bin/

# Or create symlinks
ln -s $(pwd)/bin/kxconfig ~/.local/bin/kxconfig
ln -s $(pwd)/bin/kxswap ~/.local/bin/kxswap

# Ensure they are executable
chmod +x ~/.local/bin/kxconfig ~/.local/bin/kxswap
```

## 🗑️ Uninstallation

To uninstall the tools:

```bash
# Uninstall scripts and configuration (keeps backups and project configs)
./scripts/uninstall.sh

# Or uninstall everything including data
./scripts/uninstall.sh --all
```

**Default behavior** (`--keep-data`):
- ✅ Removes installed scripts (`kxconfig`, `kxswap`)
- ✅ Removes completion script
- ✅ Removes shell configuration entries
- ✅ **Keeps** backups in `~/.kube/backups/`
- ✅ **Keeps** project configs (`~/.kube/config-<name>`)

**Full removal** (`--all`):
- ⚠️ Removes everything including backups and project configs
- ⚠️ Requires confirmation before proceeding

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

#### Replace a master kubeconfig

**Note**: This operation replaces the entire project master file (`~/.kube/config-<name>`), not merges with it.

```bash
# Replace master kubeconfig (choose project interactively)
kxconfig -x new-kubeconfig.yaml

# Replace master kubeconfig in a specific project
kxconfig -p dev -x new-kubeconfig.yaml
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
# Merge + Remove
kxconfig -p dev -m kubeconfig-new.yaml -d unwanted-context

# Replace + Remove
kxconfig -p dev -x kubeconfig-new.yaml -d unwanted-context
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

## 📁 Project Structure

The repository is organized as follows:

```
kxconfig/
├── bin/                              # Main executable scripts
│   ├── kxconfig                      # Main kubeconfig management tool
│   └── kxswap                        # Quick config swap tool
├── completions/                      # Shell completion scripts
│   └── kxconfig-completion.bash
├── scripts/                          # Installation/uninstallation scripts
│   ├── install.sh                    # Installation script
│   └── uninstall.sh                  # Uninstallation script
├── docs/                             # Documentation
│   └── CONTRIBUTING.md               # Contribution guidelines
├── LICENSE                           # License file
├── README.md                         # This file
└── release-please-config.json        # Release automation config
```

## 📁 Kubeconfig File Structure

`kxconfig` organizes kubeconfigs as follows:

```
~/.kube/
├── config              # Active kubeconfig (used by kubectl)
├── config-dev          # Master for "dev" project
├── config-prod         # Master for "prod" project
├── config-staging      # Master for "staging" project
└── backups/
    ├── config-initial.bak  # Backup of ~/.kube/config from first usage
    ├── dev.bak         # Backup for "dev" project
    ├── prod.bak        # Backup for "prod" project
    └── staging.bak     # Backup for "staging" project
```

## 📂 Managing Config Files

### Understanding the File Structure

The tool manages two types of kubeconfig files:

1. **Active Config** (`~/.kube/config`): The kubeconfig file currently used by `kubectl`. This is the file that gets read when you run `kubectl` commands.

2. **Project Masters** (`~/.kube/config-<name>`): Master kubeconfig files organized by project. These are managed by `kxconfig` and contain all contexts for a specific project.

### How Files Are Managed

#### First Time Usage

When you use `kxconfig` for the first time (when no projects exist yet):
- ✅ **Automatic backup**: If `~/.kube/config` exists, it will be automatically backed up to `~/.kube/backups/config-initial.bak`
- This ensures you can always restore your original configuration

#### Creating Projects

When creating a new project:
- A new master file is created: `~/.kube/config-<project-name>`
- The source can be:
  - A kubeconfig file you provide (via `-m` or `-x`)
  - Or the current `~/.kube/config` (if you want to import it)

#### Working with Projects

- **`kxconfig`** operates on project master files (`~/.kube/config-<name>`)
- **`kxswap`** copies a project master to the active config (`~/.kube/config`)
- Changes made with `kxconfig` are **not** automatically applied to the active config unless you use `kxswap`

### Backup Strategy

The tool maintains backups in `~/.kube/backups/`:

- **`config-initial.bak`**: Created once on first usage, contains your original `~/.kube/config`
- **`<project>.bak`**: Created/updated before each modification to a project master file
- Backups are **unique per project** (they overwrite the previous backup)

### Restoring from Backups

If something goes wrong, you can restore from backups:

```bash
# Restore the initial config
cp ~/.kube/backups/config-initial.bak ~/.kube/config

# Restore a specific project
cp ~/.kube/backups/dev.bak ~/.kube/config-dev

# Then apply it
kxswap dev
```

### Best Practices

1. **Keep your original config safe**: The initial backup (`config-initial.bak`) is created only once. Keep it safe!

2. **Use descriptive project names**: Choose clear project names (e.g., `dev`, `prod`, `staging`) to easily identify them

3. **Regular backups**: While the tool creates automatic backups, consider making manual backups before major changes:
   ```bash
   cp ~/.kube/config-dev ~/.kube/config-dev.manual-backup-$(date +%Y%m%d)
   ```

4. **Verify before swapping**: Always verify a project config before making it active:
   ```bash
   kubectl --kubeconfig=~/.kube/config-dev config get-contexts
   kxswap dev
   ```

5. **Clean up old projects**: Remove project files you no longer need:
   ```bash
   rm ~/.kube/config-old-project
   rm ~/.kube/backups/old-project.bak
   ```

## 🔧 Use Case Examples

### Case 1: Add a new cluster to the development project

```bash
# Download the new cluster's kubeconfig
kubectl --kubeconfig=novo-cluster.yaml config view --flatten > novo-cluster.yaml

# Add to dev project
kxconfig -p dev novo-cluster.yaml
```

### Case 2: Replace a project's master kubeconfig

```bash
# Replace the entire master kubeconfig for a project
kxconfig -p dev -x new-kubeconfig.yaml
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

1. **Backups**: 
   - `kxconfig` automatically creates backups in `~/.kube/backups/<project>.bak` before any changes to project masters
   - On first usage, if `~/.kube/config` exists, it's backed up to `~/.kube/backups/config-initial.bak`
   - Backups are unique per project (they overwrite the previous one)

2. **Validation**: All kubeconfigs are validated before processing using `kubectl config view`.

3. **Security**: Kubeconfig files contain sensitive credentials. Make sure you have appropriate permissions configured.

4. **Integration with kxswap**: All `kxconfig` operations are performed on master files (`~/.kube/config-<name>`). If `kxswap` is available in PATH, `kxconfig` will automatically try to apply these changes to the active kubeconfig (`~/.kube/config`) after completing operations.

5. **File Management**: The active config (`~/.kube/config`) and project masters (`~/.kube/config-<name>`) are separate files. Changes to project masters don't affect the active config until you use `kxswap`.

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

Contributions are welcome! Please read [CONTRIBUTING.md](docs/CONTRIBUTING.md) for more details on how to contribute.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Inspired by the needs of managing multiple Kubernetes clusters
- Based on best practices for kubeconfig management

## 📝 Changelog

The changelog is automatically generated from commits following the [Conventional Commits](https://www.conventionalcommits.org/) format.

See [releases](https://github.com/nunoarmada/kxconfig/releases) for the complete version history.

### Version 1.0.0
- Initial functionality
- Support for merge, replace and delete contexts
- Integration with kxswap
- Automatic backups
- Initial config backup on first usage

---

**Developed with ❤️ for the Kubernetes community**
