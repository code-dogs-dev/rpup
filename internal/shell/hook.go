// Package shell generates the shell integration that sources rpup.
package shell

import (
	"fmt"
	"strings"
)

// Supported reports whether a shell name has a hook template.
func Supported(shell string) bool {
	return shell == "zsh" || shell == "bash"
}

// Hook returns the shell code to be eval'd from a shell rc file. It defines the
// rpup function, caches the home default ruby in RPUP_DEFAULT_VERSION (read once), and
// wires per-directory auto-switching whose hot path is pure shell — the rpup
// binary is only invoked on an actual version change.
func Hook(shell string) (string, error) {
	if !Supported(shell) {
		return "", fmt.Errorf("rpup: unsupported shell: %s (want zsh or bash)", shell)
	}
	var b strings.Builder
	b.WriteString(common)
	if shell == "zsh" {
		b.WriteString(zshWiring)
	} else {
		b.WriteString(bashWiring)
	}
	return b.String(), nil
}

const common = `export RPUP_DEFAULT_VERSION="${RPUP_DEFAULT_VERSION:-$(cat "$HOME/.ruby-version" 2>/dev/null)}"

rpup() {
  case "$1" in
    use|reset)
      eval "$(command rpup "$@")" ;;
    *)
      command rpup "$@" ;;
  esac
}

_rpup_auto() {
  local dir="$PWD" version=""
  while [ -n "$dir" ] && [ "$dir" != "$HOME" ]; do
    if [ -r "$dir/.ruby-version" ]; then
      read -r version < "$dir/.ruby-version"
      break
    fi
    dir="${dir%/*}"
  done
  [ -z "$version" ] && version="$RPUP_DEFAULT_VERSION"
  [ "$version" = "$RPUP_CURRENT_VERSION" ] && return
  RPUP_CURRENT_VERSION="$version"
  eval "$(command rpup use "$version")"
}

_rpup_auto
`

const zshWiring = `if [ -n "$ZSH_VERSION" ]; then
  autoload -Uz add-zsh-hook 2>/dev/null
  if typeset -f add-zsh-hook >/dev/null; then
    add-zsh-hook chpwd _rpup_auto
  else
    case "$preexec_functions" in
      *_rpup_auto*) ;;
      *) preexec_functions+=(_rpup_auto) ;;
    esac
  fi
fi
`

const bashWiring = `if [ -n "$BASH_VERSION" ]; then
  case "$PROMPT_COMMAND" in
    *_rpup_auto*) ;;
    *) PROMPT_COMMAND="_rpup_auto${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
  esac
fi
`
