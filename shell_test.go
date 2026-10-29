package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFunctionDocs(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("README.md", "| Command | Description |\n"+
		"| ------- | ----------- |\n"+
		"| `pfs` | Return the current Finder selection |\n"+
		"| `grename <old> <new>` | Renames branch `<old>` to `<new>` |\n"+
		"| `gst` | `git status` |\n"+
		"| `gdv` | `git diff -w \"$@\" | view -` |\n")
	src := write("plugin.zsh", "#!/bin/zsh\n"+
		"# Get the value of an alias.\n"+
		"#\n"+
		"# Arguments:\n"+
		"#   1. alias\n"+
		"function alias_value() {\n}\n\n"+
		"# shellcheck disable=SC2034\n"+
		"# Make a directory and cd into it\n"+
		"mkcd () {\n}\n\n"+
		"undocumented() {\n}\n")

	docs := docCache{}
	readme := docs.readme(dir)
	for name, want := range map[string]string{
		"pfs":     "Return the current Finder selection",
		"grename": "Renames branch `<old>` to `<new>`",
		"gst":     "", // just the code it runs
		"gdv":     "", // code cut short by the "|" inside it
	} {
		if got := readme[name]; got != want {
			t.Errorf("readme[%q] = %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]string{
		"alias_value":  "Get the value of an alias.",
		"mkcd":         "Make a directory and cd into it",
		"undocumented": "",
		"missing":      "",
	} {
		if got := docs.comment(src, name); got != want {
			t.Errorf("comment(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAffixes(t *testing.T) {
	for name, want := range map[string][2]string{
		"git_prompt_info": {"git", "info"},
		"zle-line-init":   {"zle", "init"},
		"take":            {"", ""},
	} {
		first, last, _ := affixes(name)
		if first != want[0] || last != want[1] {
			t.Errorf("affixes(%q) = %q, %q, want %q, %q", name, first, last, want[0], want[1])
		}
	}
}

func TestShellUnquote(t *testing.T) {
	for in, want := range map[string]string{
		`'git status'`:                       `git status`,
		`ls`:                                 `ls`,
		`'awk '\''{print $1}'\'' file'`:      `awk '{print $1}' file`,
		`$'line one\n  it\'s two'`:           "line one\n  it's two",
		`"say \"hi\" to $USER"`:              `say "hi" to $USER`,
		`grep\ -E`:                           `grep -E`,
		`'LANG=C git branch -vv | grep ":"'`: `LANG=C git branch -vv | grep ":"`,
	} {
		if got, ok := shellUnquote(in); !ok || got != want {
			t.Errorf("shellUnquote(%s) = %q, %v, want %q", in, got, ok, want)
		}
	}
	if _, ok := shellUnquote(`'unfinished`); ok {
		t.Error("open quote reported complete")
	}
}
