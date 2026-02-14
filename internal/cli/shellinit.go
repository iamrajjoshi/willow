package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

const bashInitScript = `# Willow shell integration
# Add to your .bashrc:
#   eval "$(willow shell-init)"

alias ww='willow'

wwn() {
  local dir
  dir="$(willow new "$@" --cd)" || return
  cd "$dir" || return
}

wwg() {
  local dir
  dir="$(willow pwd "$@")" || return
  cd "$dir" || return
}

# Tab completion
__willow_init_completion() {
  COMPREPLY=()
  _get_comp_words_by_ref "$@" cur prev words cword
}

__willow_bash_autocomplete() {
  if [[ "${COMP_WORDS[0]}" != "source" ]]; then
    local cur opts base words
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    if declare -F _init_completion >/dev/null 2>&1; then
      _init_completion -n "=:" || return
    else
      __willow_init_completion -n "=:" || return
    fi
    words=("${words[@]:0:$cword}")
    if [[ "$cur" == "-"* ]]; then
      requestComp="${words[*]} ${cur} --generate-shell-completion"
    else
      requestComp="${words[*]} --generate-shell-completion"
    fi
    opts=$(eval "${requestComp}" 2>/dev/null)
    COMPREPLY=($(compgen -W "${opts}" -- ${cur}))
    return 0
  fi
}

complete -o bashdefault -o default -o nospace -F __willow_bash_autocomplete willow
complete -o bashdefault -o default -o nospace -F __willow_bash_autocomplete ww
`

const zshInitScript = `# Willow shell integration
# Add to your .zshrc:
#   eval "$(willow shell-init)"

alias ww='willow'

wwn() {
  local dir
  dir="$(willow new "$@" --cd)" || return
  cd "$dir" || return
}

wwg() {
  local dir
  dir="$(willow pwd "$@")" || return
  cd "$dir" || return
}

# Tab completion
_willow() {
  local -a opts
  local current
  current=${words[-1]}
  if [[ "$current" == "-"* ]]; then
    opts=("${(@f)$(${words[@]:0:#words[@]-1} ${current} --generate-shell-completion)}")
  else
    opts=("${(@f)$(${words[@]:0:#words[@]-1} --generate-shell-completion)}")
  fi

  if [[ "${opts[1]}" != "" ]]; then
    _describe 'values' opts
  else
    _files
  fi
}

compdef _willow willow
compdef _willow ww

if [ "$funcstack[1]" = "_willow" ]; then
  _willow
fi
`

const fishInitScript = `# Willow shell integration
# Add to your config.fish:
#   willow shell-init | source

alias ww='willow'

function wwn
  set -l dir (willow new $argv --cd)
  or return
  cd $dir
end

function wwg
  set -l dir (willow pwd $argv)
  or return
  cd $dir
end

# Tab completion
function __fish_willow_complete
  set -l tokens (commandline -opc)
  set -l cur (commandline -ct)
  if string match -q -- '-*' $cur
    $tokens $cur --generate-shell-completion 2>/dev/null
  else
    $tokens --generate-shell-completion 2>/dev/null
  end
end

complete -c willow -f -a '(__fish_willow_complete)'
complete -c ww -w willow
`

func detectShell() string {
	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "bash", "zsh", "fish":
		return shell
	default:
		return "bash"
	}
}

func shellInitCmd() *cli.Command {
	return &cli.Command{
		Name:  "shell-init",
		Usage: "Print shell integration script",
		Action: func(_ context.Context, cmd *cli.Command) error {
			shell := detectShell()
			switch shell {
			case "zsh":
				fmt.Print(zshInitScript)
			case "fish":
				fmt.Print(fishInitScript)
			default:
				fmt.Print(bashInitScript)
			}
			return nil
		},
	}
}
