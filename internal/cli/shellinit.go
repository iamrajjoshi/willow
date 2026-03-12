package cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

const bashTabTitle = `
# Set terminal tab title to willow worktree name
__willow_tab_title() {
  local wt_dir="$HOME/.willow/worktrees"
  local resolved_wt_dir resolved_pwd
  resolved_wt_dir="$(cd "$wt_dir" 2>/dev/null && pwd -P)" || return
  resolved_pwd="$(pwd -P)"
  case "$resolved_pwd" in
    "$resolved_wt_dir"/*)
      local rel="${resolved_pwd#"$resolved_wt_dir"/}"
      local repo="${rel%%/*}"
      local branch="${rel#*/}"
      branch="${branch%%/*}"
      printf '\e]0;%s/%s\a' "$repo" "$branch"
      ;;
  esac
}
PROMPT_COMMAND="__willow_tab_title;${PROMPT_COMMAND:-}"
`

const bashInitScript = `# Willow shell integration
# Add to your .bashrc:
#   eval "$(willow shell-init)"

ww() {
  if [ "$1" = "sw" ]; then
    local dir
    dir="$(command willow sw "${@:2}")" || return
    if [ -n "$TMUX" ] && [ -n "$dir" ]; then
      command willow tmux sw "$dir"
      return
    fi
    cd "$dir" || return
    return
  fi
  if [ "$1" = "rm" ]; then
    local cwd="$PWD"
    command willow "$@"
    local ret=$?
    if [ $ret -eq 0 ] && ! [ -d "$cwd" ]; then
      cd "${cwd%/*}" 2>/dev/null || cd ~/.willow/worktrees 2>/dev/null || true
    fi
    return $ret
  fi
  command willow "$@"
}

wwn() {
  local dir
  dir="$(willow new "$@" --cd)" || return
  cd "$dir" || return
}

www() { cd ~/.willow/worktrees || return; }

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

const zshTabTitle = `
# Set terminal tab title to willow worktree name
__willow_tab_title() {
  local wt_dir="$HOME/.willow/worktrees"
  local resolved_wt_dir resolved_pwd
  resolved_wt_dir="$(cd "$wt_dir" 2>/dev/null && pwd -P)" || return
  resolved_pwd="$(pwd -P)"
  case "$resolved_pwd" in
    "$resolved_wt_dir"/*)
      local rel="${resolved_pwd#"$resolved_wt_dir"/}"
      local repo="${rel%%/*}"
      local branch="${rel#*/}"
      branch="${branch%%/*}"
      printf '\e]0;%s/%s\a' "$repo" "$branch"
      ;;
  esac
}
precmd_functions+=(__willow_tab_title)
`

const zshInitScript = `# Willow shell integration
# Add to your .zshrc:
#   eval "$(willow shell-init)"

ww() {
  if [ "$1" = "sw" ]; then
    local dir
    dir="$(command willow sw "${@:2}")" || return
    if [ -n "$TMUX" ] && [ -n "$dir" ]; then
      command willow tmux sw "$dir"
      return
    fi
    cd "$dir" || return
    return
  fi
  if [ "$1" = "rm" ]; then
    local cwd="$PWD"
    command willow "$@"
    local ret=$?
    if [ $ret -eq 0 ] && ! [ -d "$cwd" ]; then
      cd "${cwd%/*}" 2>/dev/null || cd ~/.willow/worktrees 2>/dev/null || true
    fi
    return $ret
  fi
  command willow "$@"
}

wwn() {
  local dir
  dir="$(willow new "$@" --cd)" || return
  cd "$dir" || return
}

www() { cd ~/.willow/worktrees || return; }

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

const fishTabTitle = `
# Set terminal tab title to willow worktree name
function __willow_tab_title --on-variable PWD
  set -l wt_dir "$HOME/.willow/worktrees"
  set -l resolved_wt_dir (cd "$wt_dir" 2>/dev/null; and pwd -P)
  set -l resolved_pwd (pwd -P)
  if string match -q "$resolved_wt_dir/*" "$resolved_pwd"
    set -l rel (string replace "$resolved_wt_dir/" "" "$resolved_pwd")
    set -l repo (string split / "$rel")[1]
    set -l branch (string split / "$rel")[2]
    printf '\e]0;%s/%s\a' "$repo" "$branch"
  end
end
`

const fishInitScript = `# Willow shell integration
# Add to your config.fish:
#   willow shell-init | source

function ww
  if test (count $argv) -gt 0; and test "$argv[1]" = "sw"
    set -l dir (command willow sw $argv[2..])
    or return
    if test -n "$TMUX"; and test -n "$dir"
      command willow tmux sw "$dir"
      return
    end
    cd $dir
    return
  end
  if test (count $argv) -gt 0; and test "$argv[1]" = "rm"
    set -l cwd $PWD
    command willow $argv
    set -l ret $status
    if test $ret -eq 0; and not test -d "$cwd"
      cd (string replace -r '/[^/]+$' '' "$cwd") 2>/dev/null
        or cd ~/.willow/worktrees 2>/dev/null
        or true
    end
    return $ret
  end
  command willow $argv
end

function wwn
  set -l dir (willow new $argv --cd)
  or return
  cd $dir
end

function www
  cd ~/.willow/worktrees; or return
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
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "tab-title",
				Usage: "Include terminal tab title integration (sets tab to repo/branch in willow worktrees)",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			shell := detectShell()
			tabTitle := cmd.Bool("tab-title")
			switch shell {
			case "zsh":
				os.Stdout.WriteString(zshInitScript)
				if tabTitle {
					os.Stdout.WriteString(zshTabTitle)
				}
			case "fish":
				os.Stdout.WriteString(fishInitScript)
				if tabTitle {
					os.Stdout.WriteString(fishTabTitle)
				}
			default:
				os.Stdout.WriteString(bashInitScript)
				if tabTitle {
					os.Stdout.WriteString(bashTabTitle)
				}
			}
			return nil
		},
	}
}
