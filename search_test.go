package main

import (
	"os"
	"path/filepath"
	"testing"
)

func testSearcher() *Searcher {
	return NewSearcher(&Index{Entries: []Entry{
		{Name: "lsof", Kind: "bin", ManDesc: "list open files"},
		{Name: "ls", Kind: "bin", ManDesc: "list directory contents"},
		{Name: "rm", Kind: "bin", ManDesc: "remove directory entries"},
		{Name: "gzip", Kind: "bin", ManDesc: "compression/decompression tool using Lempel-Ziv coding"},
		{Name: "export", Kind: "builtin", ManDesc: "shell built-in commands"},
		{Name: "rsync", Kind: "bin", ManDesc: "synchronise local and remote files"},
		{Name: "jq", Kind: "bin", Pkg: "jq", PkgDesc: "Lightweight and flexible command-line JSON processor"},
		{Name: "pbcopy", Kind: "bin", ManDesc: "provide copying and pasting to the pasteboard"},
	}})
}

func top(s *Searcher, q string) string {
	r := s.Search(q, 1)
	if len(r) == 0 {
		return ""
	}
	return r[0].Entry.Name
}

func TestSearchRanking(t *testing.T) {
	s := testSearcher()
	cases := map[string]string{
		"open files list":          "lsof",  // order doesn't matter
		"lsof":                     "lsof",  // exact name
		"delete":                   "rm",    // synonym
		"compress":                 "gzip",  // prefix of "compression"
		"synchronize files remote": "rsync", // typo (z vs s)
		"json":                     "jq",
		"clipboard":                "pbcopy",
		"list open fil":            "lsof", // partial last word while typing
	}
	for q, want := range cases {
		if got := top(s, q); got != want {
			t.Errorf("Search(%q) top = %q, want %q", q, got, want)
		}
	}
}

func TestStopwordsDontExclude(t *testing.T) {
	if got := top(testSearcher(), "how do I list the open files"); got != "lsof" {
		t.Errorf("got %q, want lsof", got)
	}
}

func TestNameSubstringIsWeak(t *testing.T) {
	// "port" is inside "export" but that shouldn't beat a real description match.
	s := NewSearcher(&Index{Entries: []Entry{
		{Name: "export", Kind: "builtin", ManDesc: "shell built-in commands"},
		{Name: "netstat", Kind: "bin", ManDesc: "show network status and port usage"},
	}})
	if got := top(s, "port"); got != "netstat" {
		t.Errorf("got %q, want netstat", got)
	}
}

func TestEditDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		d    int
	}{
		{"search", "search", 0},
		{"serach", "search", 1}, // transposition
		{"synchronize", "synchronise", 1},
		{"kitten", "sitting", 3},
	} {
		if got := editDistance(c.a, c.b); got != c.d {
			t.Errorf("editDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.d)
		}
	}
}

func TestParseManFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "man1")
	os.MkdirAll(dir, 0o755)
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(body), 0o644)
		return p
	}

	man := write("gzip.1", `.TH GZIP 1
.SH NAME
gzip, gunzip, zcat \- compress or expand files
.SH SYNOPSIS
`)
	mdoc := write("ls.1", `.Dd May 1, 2020
.Dt LS 1
.Os
.Sh NAME
.Nm ls ,
.Nm gls
.Nd list directory contents
.Sh SYNOPSIS
`)
	pod := write("perl.1", `.IX Title "PERL 1"
.SH "NAME"
.IX Header "NAME"
perl \- The Perl 5 language \fIinterpreter\fR
.SH "SYNOPSIS"
`)
	so := write("zcat.1", ".so man1/gzip.1\n")

	for _, c := range []struct {
		path  string
		names []string
		desc  string
	}{
		{man, []string{"gzip", "gunzip", "zcat"}, "compress or expand files"},
		{mdoc, []string{"ls", "gls"}, "list directory contents"},
		{pod, []string{"perl"}, "The Perl 5 language interpreter"},
		{so, []string{"gzip", "gunzip", "zcat"}, "compress or expand files"},
	} {
		names, desc := parseManFile(root, c.path, 0)
		if desc != c.desc {
			t.Errorf("%s: desc = %q, want %q", filepath.Base(c.path), desc, c.desc)
		}
		if len(names) != len(c.names) {
			t.Errorf("%s: names = %q, want %q", filepath.Base(c.path), names, c.names)
			continue
		}
		for i := range names {
			if names[i] != c.names[i] {
				t.Errorf("%s: names = %q, want %q", filepath.Base(c.path), names, c.names)
				break
			}
		}
	}
}
