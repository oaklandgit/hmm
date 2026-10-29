package main

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry is one runnable command and everything we know about it.
type Entry struct {
	Name    string // what you type
	Path    string // resolved location ("" for builtins/aliases)
	Kind    string // "bin", "builtin", "alias"
	ManDesc string // one-line summary from the man page NAME section
	ManSect string // man section the summary came from
	Pkg     string // owning Homebrew formula/cask, if any
	PkgDesc string // Homebrew description
	Extra   string // alias expansion, etc.
}

// Desc is the best single description for display.
func (e *Entry) Desc() string {
	switch {
	case e.ManDesc != "":
		return e.ManDesc
	case e.PkgDesc != "":
		return e.PkgDesc
	case e.Extra != "":
		return e.Extra
	}
	return ""
}

type Index struct {
	Version int
	Built   time.Time
	PathEnv string
	Entries []Entry
}

const indexVersion = 1

// staleAfter forces a periodic rebuild even if nothing obvious changed,
// since man pages and brew descriptions can update without touching PATH dirs.
const staleAfter = 7 * 24 * time.Hour

func indexFile() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "hmm", "index.gob")
}

func loadIndex() (*Index, error) {
	f, err := os.Open(indexFile())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var idx Index
	if err := gob.NewDecoder(f).Decode(&idx); err != nil {
		return nil, err
	}
	if idx.Version != indexVersion {
		return nil, fmt.Errorf("index version %d, want %d", idx.Version, indexVersion)
	}
	return &idx, nil
}

func (idx *Index) save() error {
	path := indexFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(idx); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// stale reports whether the index no longer reflects the system: PATH
// changed, a PATH directory was modified (something installed or removed),
// or it's simply old.
func (idx *Index) stale() bool {
	if idx.PathEnv != os.Getenv("PATH") || time.Since(idx.Built) > staleAfter {
		return true
	}
	for _, dir := range filepath.SplitList(idx.PathEnv) {
		if fi, err := os.Stat(dir); err == nil && fi.ModTime().After(idx.Built) {
			return true
		}
	}
	return false
}

// loadOrBuild returns a fresh-enough index, rebuilding it if needed.
func loadOrBuild(force bool) (*Index, error) {
	if !force {
		if idx, err := loadIndex(); err == nil && !idx.stale() {
			return idx, nil
		}
	}
	fmt.Fprint(os.Stderr, "hmm: indexing commands…")
	start := time.Now()
	idx := buildIndex()
	fmt.Fprintf(os.Stderr, "\r\033[Khmm: indexed %d commands in %s\n",
		len(idx.Entries), time.Since(start).Round(time.Millisecond))
	if err := idx.save(); err != nil {
		fmt.Fprintln(os.Stderr, "hmm: could not save index:", err)
	}
	return idx, nil
}

func buildIndex() *Index {
	var (
		wg       sync.WaitGroup
		bins     map[string]string
		man      map[string]manInfo
		brew     map[string]string
		builtins []string
		aliases  map[string]string
	)
	wg.Add(5)
	go func() { defer wg.Done(); bins = scanPath() }()
	go func() { defer wg.Done(); man = scanMan() }()
	go func() { defer wg.Done(); brew = brewDescs() }()
	go func() { defer wg.Done(); builtins = shellBuiltins() }()
	go func() { defer wg.Done(); aliases = shellAliases() }()
	wg.Wait()

	entries := make([]Entry, 0, len(bins)+len(builtins)+len(aliases))
	seen := map[string]bool{}
	add := func(e Entry) {
		if mi, ok := man[e.Name]; ok {
			e.ManDesc, e.ManSect = mi.desc, mi.section
		}
		seen[e.Name] = true
		entries = append(entries, e)
	}

	// Aliases and builtins shadow binaries in the shell, so list them first.
	for name, exp := range aliases {
		add(Entry{Name: name, Kind: "alias", Extra: exp})
	}
	for _, name := range builtins {
		if !seen[name] {
			add(Entry{Name: name, Kind: "builtin"})
		}
	}
	for name, path := range bins {
		if seen[name] {
			continue
		}
		e := Entry{Name: name, Path: path, Kind: "bin"}
		if pkg := brewOwner(path); pkg != "" {
			e.Pkg, e.PkgDesc = pkg, brew[pkg]
		}
		add(e)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return &Index{
		Version: indexVersion,
		Built:   time.Now(),
		PathEnv: os.Getenv("PATH"),
		Entries: entries,
	}
}

// scanPath returns name -> path for every executable reachable via PATH,
// honoring PATH order (first match wins, as in the shell).
func scanPath() map[string]string {
	bins := map[string]string{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range ents {
			name := de.Name()
			if _, ok := bins[name]; ok || strings.HasPrefix(name, ".") {
				continue
			}
			full := filepath.Join(dir, name)
			fi, err := os.Stat(full) // follows symlinks
			if err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
				continue
			}
			bins[name] = full
		}
	}
	return bins
}
