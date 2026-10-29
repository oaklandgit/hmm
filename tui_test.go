package main

import "testing"

func TestDetails(t *testing.T) {
	for _, tc := range []struct {
		e    Entry
		want string
	}{
		{Entry{Name: "cal", Kind: "bin", Path: "/usr/bin/cal", ManSect: "1", ManDesc: "displays a calendar"},
			"system: /usr/bin/cal · man cal(1)"},
		{Entry{Name: "gst", Kind: "alias", Extra: "alias for: git status"},
			"alias for: git status"},
		{Entry{Name: "pfs", Kind: "function", Path: "/Users/x/.oh-my-zsh/plugins/macos/macos.plugin.zsh", Extra: "Return the current Finder selection"},
			"function: /Users/x/.oh-my-zsh/plugins/macos/macos.plugin.zsh"},
		{Entry{Name: "ffprobe", Kind: "bin", Path: "/opt/homebrew/bin/ffprobe", Pkg: "ffmpeg", PkgDesc: "Play, record, convert, and stream audio and video"},
			"brew: /opt/homebrew/bin/ffprobe · from ffmpeg"},
		{Entry{Name: "lame", Kind: "bin", Path: "/opt/homebrew/bin/lame", Pkg: "lame", ManSect: "1",
			ManDesc: "create mp3 audio files", PkgDesc: "High quality MPEG Audio Layer III (MP3) encoder"},
			"brew: /opt/homebrew/bin/lame · man lame(1)"},
		{Entry{Name: "export", Kind: "builtin"},
			"builtin"},
	} {
		if got := details(&tc.e); got != tc.want {
			t.Errorf("details(%s) = %q, want %q", tc.e.Name, got, tc.want)
		}
	}
}
