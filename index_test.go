package main

import "testing"

func TestSource(t *testing.T) {
	for _, tc := range []struct {
		e    Entry
		want string
	}{
		{Entry{Kind: "alias"}, "alias"},
		{Entry{Kind: "function", Path: "/Users/x/.oh-my-zsh/lib/functions.zsh"}, "function"},
		{Entry{Kind: "bin", Path: "/usr/bin/lsof"}, "system"},
		{Entry{Kind: "bin", Path: "/sbin/ping"}, "system"},
		{Entry{Kind: "bin", Path: "/opt/homebrew/bin/jq", Pkg: "jq"}, "brew"},
		{Entry{Kind: "bin", Path: "/opt/homebrew/bin/brew"}, "brew"},
		{Entry{Kind: "bin", Path: "/Users/x/.cargo/bin/rustc"}, "cargo"},
		{Entry{Kind: "bin", Path: "/Users/x/go/bin/hmm"}, "go"},
		{Entry{Kind: "bin", Path: "/Users/x/.nvm/versions/node/v24.11.1/bin/npx"}, "npm"},
		{Entry{Kind: "bin", Path: "/Users/x/Library/Python/3.9/bin/pip3"}, "pip"},
		{Entry{Kind: "bin", Path: "/Applications/love.app/Contents/MacOS/love"}, "app"},
		{Entry{Kind: "bin", Path: "/usr/local/bin/code"}, "local"},
		{Entry{Kind: "bin", Path: "/snap/bin/firefox"}, "snap"},
	} {
		if got := tc.e.Source(); got != tc.want {
			t.Errorf("Source(%s) = %q, want %q", tc.e.Path, got, tc.want)
		}
	}
}
