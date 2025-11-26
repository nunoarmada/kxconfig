#!/usr/bin/env bash
# Completion script for kxconfig and kxswap
# Supports both bash and zsh

_kxconfig_complete() {
  local cur prev words
  local commands="-h --help -p --project -m --merge -x --replace -d --delete-context"
  local projects=()

  # Get current word and previous word
  if [ -n "${BASH_VERSION:-}" ]; then
    # Bash completion
    _get_comp_words_by_ref -n : cur prev words 2>/dev/null || return
  elif [ -n "${ZSH_VERSION:-}" ]; then
    # Zsh completion
    cur="${words[CURRENT]}"
    prev="${words[CURRENT-1]}"
  fi

  # Get list of projects from ~/.kube/config-*
  if [ -d "${HOME}/.kube" ]; then
    for f in "${HOME}"/.kube/config-*; do
      [ -f "$f" ] || continue
      local name="${f##*/config-}"
      projects+=("$name")
    done
  fi

  # Complete project names after -p/--project
  if [[ "$prev" == "-p" ]] || [[ "$prev" == "--project" ]]; then
    if [ -n "${BASH_VERSION:-}" ]; then
      # shellcheck disable=SC2207
      COMPREPLY=($(compgen -W "${projects[*]}" -- "$cur"))
    elif [ -n "${ZSH_VERSION:-}" ]; then
      compadd -a projects
    fi
    return 0
  fi

  # Complete file paths after -m/--merge or -x/--replace
  if [[ "$prev" == "-m" ]] || [[ "$prev" == "--merge" ]] || \
     [[ "$prev" == "-x" ]] || [[ "$prev" == "--replace" ]]; then
    if [ -n "${BASH_VERSION:-}" ]; then
      _filedir
    elif [ -n "${ZSH_VERSION:-}" ]; then
      _files
    fi
    return 0
  fi

  # Complete context names after -d/--delete-context
  if [[ "$prev" == "-d" ]] || [[ "$prev" == "--delete-context" ]]; then
    # Try to get contexts from current kubeconfig if available
    if command -v kubectl >/dev/null 2>&1; then
      local contexts
      contexts=$(kubectl config get-contexts -o name 2>/dev/null || true)
      if [ -n "$contexts" ]; then
        if [ -n "${BASH_VERSION:-}" ]; then
          # shellcheck disable=SC2207
          COMPREPLY=($(compgen -W "$contexts" -- "$cur"))
        elif [ -n "${ZSH_VERSION:-}" ]; then
          # Convert newline-separated to array for zsh
          local contexts_array=()
          while IFS= read -r line; do
            [ -n "$line" ] && contexts_array+=("$line")
          done <<< "$contexts"
          compadd -a contexts_array
        fi
        return 0
      fi
    fi
    return 0
  fi

  # Complete options or file paths for positional argument
  if [[ "$cur" == -* ]]; then
    if [ -n "${BASH_VERSION:-}" ]; then
      # shellcheck disable=SC2207
      COMPREPLY=($(compgen -W "$commands" -- "$cur"))
    elif [ -n "${ZSH_VERSION:-}" ]; then
      # shellcheck disable=SC2086,SC2206,SC2034
      local cmd_array=(${=commands})
      compadd -a cmd_array
    fi
  else
    # Check if we're expecting a file (no -m or -x seen yet)
    local has_file_op=false
    local word
    for word in "${words[@]}"; do
      if [[ "$word" == "-m" ]] || [[ "$word" == "--merge" ]] || \
         [[ "$word" == "-x" ]] || [[ "$word" == "--replace" ]]; then
        has_file_op=true
        break
      fi
    done

    if [ "$has_file_op" = false ]; then
      if [ -n "${BASH_VERSION:-}" ]; then
        _filedir
      elif [ -n "${ZSH_VERSION:-}" ]; then
        _files
      fi
    fi
  fi

  return 0
}

_kxswap_complete() {
  local cur prev words
  local projects=()

  # Get current word
  if [ -n "${BASH_VERSION:-}" ]; then
    # Bash completion
    _get_comp_words_by_ref -n : cur prev words 2>/dev/null || return
  elif [ -n "${ZSH_VERSION:-}" ]; then
    # Zsh completion
    cur="${words[CURRENT]}"
  fi

  # Get list of projects from ~/.kube/config-*
  if [ -d "${HOME}/.kube" ]; then
    for f in "${HOME}"/.kube/config-*; do
      [ -f "$f" ] || continue
      local name="${f##*/config-}"
      projects+=("$name")
    done
  fi

  # Complete project names
  if [ -n "${BASH_VERSION:-}" ]; then
    # shellcheck disable=SC2207
    COMPREPLY=($(compgen -W "${projects[*]}" -- "$cur"))
  elif [ -n "${ZSH_VERSION:-}" ]; then
    compadd -a projects
  fi
  return 0
}

# Register completions
if [ -n "${BASH_VERSION:-}" ]; then
  # Bash completion
  complete -F _kxconfig_complete kxconfig
  complete -F _kxswap_complete kxswap
elif [ -n "${ZSH_VERSION:-}" ]; then
  # Zsh completion
  compdef _kxconfig_complete kxconfig
  compdef _kxswap_complete kxswap
fi

