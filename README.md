# hmm

Find the command you're thinking of by describing it.

```
$ hmm -l list open files
lsof                     list open files
fuser                    list process IDs of all processes that have one or more files open
open                     open files and directories
```

hmm indexes every executable your shell can reach and attaches whatever
description it can find, so a few keywords in any order narrow it down:

| Source          | What it contributes                                              |
|-----------------|------------------------------------------------------------------|
| `$PATH`         | every executable, in shell precedence order                      |
| man pages       | the NAME-section summary (man(7) and mdoc(7), gzipped, `.so`)    |
| Homebrew        | formula/cask description, matched by resolving the Cellar symlink |
| your shell      | builtins and reserved words (zsh, bash, fish), plus aliases      |

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

In the finder: type to filter, ↑/↓ to move, **enter** prints the command to
stdout, **ctrl+o** opens its man page, **esc** quits.

### Shell keybinding

```sh
# ~/.zshrc  (or --init bash in ~/.bashrc)
eval "$(hmm --init zsh)"
```

Press **Ctrl-G** to open the finder seeded with whatever you've typed, and the
chosen command replaces the line.

## How matching works

- Every query word must hit the name or a description *somewhere*, in any
  order. Results that miss words still show, but rank well below full matches.
- Name matches beat description matches; rare words count more than common ones.
- Forgiving: prefixes (`compress` → *compression*), stems (`deleting` →
  *delete*), one or two typos, and a small table of synonyms
  (`delete` ↔ *remove*, `clipboard` ↔ *pasteboard*, …).
- Filler words ("how do I…") are ignored.

## Index

Stored at `~/Library/Caches/hmm/index.gob` (or `$XDG_CACHE_HOME`). It's
rebuilt automatically when `$PATH` changes, when anything is installed into or
removed from a `$PATH` directory, or after a week. Building takes about a second.
