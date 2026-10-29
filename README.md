# hmm

Solves the problem of "hmm, what was that CLI command I'm looking for?" Now you can just start describing it.

![demo](demo.gif)

hmm indexes every executable your shell can reach and attaches whatever
description it can find, so a few keywords in any order narrow it down:

| Source     | What it contributes                                                                       |
| ---------- | ----------------------------------------------------------------------------------------- |
| `$PATH`    | every executable, in shell precedence order                                               |
| man pages  | the NAME-section summary (man(7) and mdoc(7), gzipped, `.so`)                             |
| Homebrew   | formula/cask description, matched by resolving the Cellar symlink                         |
| your shell | builtins and reserved words (zsh, bash, fish), plus aliases                               |
| functions  | shell functions, described by fish, an Oh My Zsh plugin's README, or a nearby comment     |

The line under the results says where the highlighted command comes from:
`system` for what came with the OS, `brew`, `alias`, `builtin`, `function`, or
the tool that installed it (`cargo`, `npm`, `pip`, `bun`, `go`, `app`,
`local`, …).

## Install

```sh
go install github.com/oaklandgit/hmm@latest
```

This puts the `hmm` binary in `$(go env GOPATH)/bin` (usually `~/go/bin`), or
in `$GOBIN` if that's set. If `hmm` isn't found afterwards, add that directory
to your `PATH`:

```sh
# ~/.zshrc  (or ~/.bashrc)
export PATH="$PATH:$(go env GOPATH)/bin"
```

Then open a new terminal and check with `which hmm`.

## Use

```sh
hmm                    # interactive finder
hmm compress image     # start with a query
hmm -l kill by name    # print matches, no UI
hmm --reindex          # force a rebuild
```

In the finder: type to filter, ↑/↓ to move, **enter** chooses the command,
**ctrl+o** opens its man page, **esc** quits.

### Man pages

Not sure a result is the right one? Press **ctrl+o** to read its man page
without leaving the finder. It runs `man` on the highlighted command, so your
usual pager (`$MANPAGER` or `$PAGER`) is used. Quit the pager (usually **q**)
to return to the finder with your query and selection as you left them.

The line under the results shows whether the highlighted command has a page,
and in which section, e.g. `system: /usr/bin/cal · man cal(1)`. On macOS,
shell builtins open the shared `builtin` page; aliases and shell functions
usually have no page, so ctrl+o just returns to the finder.

### Shell integration

```sh
# ~/.zshrc  (or --init bash in ~/.bashrc)
eval "$(hmm --init zsh)"
```

With this in zsh, the command you choose appears on your next prompt, ready to
run or to add arguments to. Without it (or in bash), hmm prints the command
instead, which also makes it easy to use in scripts: `cmd=$(hmm)`.

In both shells, **Ctrl-G** opens the finder seeded with whatever you've typed,
and the chosen command replaces the line.

## How matching works

- Every query word must hit the name or a description _somewhere_, in any
  order. Results that miss words still show, but rank well below full matches.
- Name matches beat description matches; rare words count more than common ones.
- Forgiving: prefixes (`compress` → _compression_), stems (`deleting` →
  _delete_), one or two typos, and a small table of synonyms
  (`delete` ↔ _remove_, `clipboard` ↔ _pasteboard_, …).
- Filler words ("how do I…") are ignored.

## Index

Stored at `~/Library/Caches/hmm/index.gob` (or `$XDG_CACHE_HOME`). It's
rebuilt automatically when `$PATH` changes, when anything is installed into or
removed from a `$PATH` directory, or after a week. Building takes about a second.
Changes to your aliases or shell functions aren't detected, so run
`hmm --reindex` after editing your shell config.
