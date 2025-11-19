# Contributing Guide

Thank you for considering contributing to `kubectx-merge`! This document provides guidelines for contributions.

## 📋 How to Contribute

### Reporting Bugs

If you find a bug, please:

1. Check if the bug hasn't already been reported in [Issues](https://github.com/seu-usuario/kubectx-merge/issues)
2. Create a new issue with:
   - Clear description of the problem
   - Steps to reproduce
   - Expected behavior vs. current behavior
   - Bash and kubectl version
   - Operating system

### Suggesting Features

We'd love to hear your ideas! To suggest a new feature:

1. Check if a similar issue doesn't already exist
2. Create a new issue with:
   - Clear description of the feature
   - Use cases and examples
   - Benefits for users

### Contributing Code

#### Environment Setup

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/your-username/kubectx-merge.git
   cd kubectx-merge
   ```

#### Development Process

1. Create a branch for your feature/fix:
   ```bash
   git checkout -b feature/my-feature
   # or
   git checkout -b fix/bug-fix
   ```

2. Make the necessary changes

3. Test your changes:
   ```bash
   # Test scripts manually
   ./kxconfig --help
   ./kxswap dev
   ```

4. Ensure the code follows conventions:
   - Use `set -euo pipefail` in bash scripts
   - Keep error messages clear and informative
   - Add comments when necessary
   - Follow existing code style

5. Commit your changes:
   ```bash
   git add .
   git commit -m "Clear description of the change"
   ```

6. Push to your fork:
   ```bash
   git push origin feature/my-feature
   ```

7. Open a Pull Request in the original repository

#### Code Conventions

- **Bash**: Use `set -euo pipefail` and `IFS=$'\n\t'`
- **Variable names**: UPPERCASE for global variables
- **Functions**: Descriptive names in lowercase with hyphens
- **Messages**: In English for user-facing messages
- **Comments**: In English, clear and concise

#### Commit Conventions

This project uses [Conventional Commits](https://www.conventionalcommits.org/) to automate releases. Please use the following format:

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Formatting, semicolons, etc. (doesn't affect code)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding or fixing tests
- `chore`: Maintenance tasks (dependencies, build, etc.)

**Examples:**
```
feat(kxconfig): add support for listing projects
fix(kxswap): fix validation of invalid kubeconfig
docs(readme): update installation instructions
```

**Note**: Commits following the conventional commits format automatically generate releases and changelogs.

#### Example Script Structure

```bash
#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

# Global variables
SCRIPT_NAME="$(basename "$0")"
BASE_DIR="${HOME}/.kube"

# Helper functions
log() {
  echo "[$SCRIPT_NAME] $*"
}

err() {
  echo "[$SCRIPT_NAME] ERROR: $*" >&2
}

# Main function
main() {
  # code here
}

main "$@"
```

### Pull Request Review

- All PRs will be reviewed
- You may be asked to make changes before merging
- Keep PRs focused on a single change when possible

## 📝 Pull Request Checklist

Before submitting a PR, ensure that:

- [ ] The code works correctly
- [ ] You tested the changes manually
- [ ] You follow code conventions
- [ ] You added comments when necessary
- [ ] You updated documentation if necessary
- [ ] Commit messages are clear and descriptive

## 🎯 Areas Where We Need Help

- Automated tests
- Documentation improvements
- Support for more operating systems
- Additional features (see issues)

## 📞 Contact

If you have questions, you can:
- Open an issue
- Create a discussion on GitHub

Thank you for contributing! 🎉
