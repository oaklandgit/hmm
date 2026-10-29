// hmm finds the command you're thinking of by describing it.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

const usage = `hmm — find any command on your system by describing it

Usage:
  hmm [query...]          interactive search (prints selection to stdout,
                          or puts it on the next prompt with --init zsh)
  hmm -l [query...]       print ranked matches and exit
  hmm --reindex           rebuild the index now
  hmm --init zsh|bash     print shell integration (Ctrl-G keybinding) to source

Examples:
  hmm compress image
  hmm -l list open files network
  eval "$(hmm --init zsh)"   # add to ~/.zshrc

Flags:
`

func main() {
	var (
		list    = flag.Bool("l", false, "print matches instead of opening the interactive picker")
		limit   = flag.Int("n", 20, "max results with -l")
		reindex = flag.Bool("reindex", false, "rebuild the command index")
		initSh  = flag.String("init", "", "print shell integration for `shell` (zsh, bash)")
	)
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *initSh != "" {
		script, ok := shellInit[*initSh]
		if !ok {
			fmt.Fprintf(os.Stderr, "hmm: no integration for %q (try zsh or bash)\n", *initSh)
			os.Exit(2)
		}
		fmt.Print(script)
		return
	}

	idx, err := loadOrBuild(*reindex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hmm:", err)
		os.Exit(1)
	}
	query := strings.Join(flag.Args(), " ")
	if *reindex && query == "" {
		return
	}
	s := NewSearcher(idx)

	if *list {
		for _, r := range s.Search(query, *limit) {
			fmt.Printf("%-24s %s\n", r.Entry.Name, r.Entry.Desc())
		}
		return
	}

	chosen, err := runTUI(s, len(idx.Entries), idx.Built, query)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hmm:", err)
		os.Exit(1)
	}
	if chosen == "" {
		os.Exit(130)
	}
	fmt.Println(chosen)
}

// shellInit binds Ctrl-G to open the picker, seeded with whatever is on the
// command line, and replaces the line with the chosen command. In zsh it
// also wraps hmm so a choice made at the prompt lands on the next prompt,
// ready to run or edit; flags and pipes get the plain command. The wrapper
// is reached through an alias so "which hmm" prints one line naming it,
// rather than the whole function body.
var shellInit = map[string]string{
	"zsh": `_hmm_prompt() {
  local a sel
  for a in "$@"; do
    [[ $a == -* ]] && { command hmm "$@"; return; }
  done
  [[ -t 1 ]] || { command hmm "$@"; return; }
  sel=$(command hmm "$@") && [[ -n $sel ]] && print -z -- "$sel "
}
alias hmm=_hmm_prompt
_hmm_widget() {
  local sel
  sel=$(command hmm -- "$BUFFER" </dev/tty 2>/dev/tty) && [[ -n $sel ]] && {
    BUFFER="$sel "
    CURSOR=${#BUFFER}
  }
  zle reset-prompt
}
zle -N _hmm_widget
bindkey '^G' _hmm_widget
`,
	"bash": `_hmm_widget() {
  local sel
  sel=$(command hmm -- "$READLINE_LINE" </dev/tty 2>/dev/tty) && [[ -n $sel ]] && {
    READLINE_LINE="$sel "
    READLINE_POINT=${#READLINE_LINE}
  }
}
bind -x '"\C-g": _hmm_widget'
`,
}
