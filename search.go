package main

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// doc is a search-ready view of an Entry.
type doc struct {
	entry     *Entry
	name      string   // lowercased command name
	nameParts []string // name split on - _ . (e.g. "git-lfs" -> git, lfs)
	words     []string // lowercased words from all descriptions
}

type Searcher struct {
	docs []doc
}

func NewSearcher(idx *Index) *Searcher {
	s := &Searcher{docs: make([]doc, len(idx.Entries))}
	for i := range idx.Entries {
		e := &idx.Entries[i]
		name := strings.ToLower(e.Name)
		s.docs[i] = doc{
			entry:     e,
			name:      name,
			nameParts: splitWords(name),
			words:     splitWords(strings.ToLower(e.ManDesc + " " + e.PkgDesc + " " + e.Pkg + " " + e.Extra)),
		}
	}
	return s
}

func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+'
	})
}

type Result struct {
	Entry *Entry
	Score float64
}

// Search ranks entries by how well they match the query terms, in any order,
// anywhere in the name or descriptions. Entries matching more of the query
// rank first; terms that are rare across the index count more than common
// ones, and name matches beat description matches.
func (s *Searcher) Search(query string, limit int) []Result {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil
	}

	type cand struct {
		d      *doc
		scores []float64
	}
	var cands []cand
	df := make([]int, len(terms))
	for i := range s.docs {
		d := &s.docs[i]
		scores := make([]float64, len(terms))
		hit := false
		for t, term := range terms {
			if scores[t] = matchTerm(d, term); scores[t] > 0 {
				df[t]++
				hit = true
			}
		}
		if hit {
			cands = append(cands, cand{d, scores})
		}
	}

	n := float64(len(s.docs))
	idf := make([]float64, len(terms))
	var idfSum float64
	for t := range terms {
		idf[t] = math.Log(1 + n/float64(df[t]+1))
		idfSum += idf[t]
	}
	results := make([]Result, len(cands))
	for i, c := range cands {
		var total, covered float64
		for t, sc := range c.scores {
			total += sc * idf[t]
			if sc > 0 {
				covered += idf[t]
			}
		}
		// Missing a query term is costly: coverage is squared so a full
		// match of mediocre quality beats a great match on half the query.
		cov := covered / idfSum
		total *= cov * cov
		e := c.d.entry
		switch {
		case e.ManDesc != "" || e.Kind == "alias" || e.Kind == "function" && e.Extra != "":
			total *= 1.15 // described in its own right
		case e.PkgDesc != "" && e.Pkg == e.Name:
			total *= 1.1 // a package's main binary
		case e.PkgDesc == "" && e.Extra == "":
			total *= 0.9 // nothing but a name to go on
		}
		results[i] = Result{e, total}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return len(results[i].Entry.Name) < len(results[j].Entry.Name)
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// queryTerms splits a query into words, dropping filler that would
// otherwise have to match something ("how do I rename many files").
func queryTerms(query string) []string {
	var terms []string
	for _, w := range splitWords(strings.ToLower(query)) {
		if !stopwords[w] {
			terms = append(terms, w)
		}
	}
	return terms
}

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the to of for in on at by with and or
		my me i how do does can what which that this these those is are be
		many some all any every from into using use via like want need`) {
		stopwords[w] = true
	}
}

// matchTerm returns how well a single query term matches a doc, or 0.
// Synonyms are tried at a discount unless the name already matched.
func matchTerm(d *doc, term string) float64 {
	best := matchLiteral(d, term)
	if best >= 55 {
		return best
	}
	for _, syn := range synonyms[term] {
		if sc := matchSynonym(d, syn) * 0.6; sc > best {
			best = sc
		}
	}
	return best
}

// matchSynonym is matchLiteral without the partial name matches: synonyms
// include short command names like "cp" and "rm", which would otherwise
// match every name that happens to start with them (cpan, rmdir).
func matchSynonym(d *doc, syn string) float64 {
	if d.name == syn {
		return 100
	}
	best := 0.0
	for _, p := range d.nameParts {
		if p == syn {
			best = 55
		}
	}
	for _, w := range d.words {
		if len(syn) < 3 {
			if w == syn {
				best = max(best, 30)
			}
			continue
		}
		best = max(best, matchWord(w, syn))
	}
	return best
}

func matchLiteral(d *doc, term string) float64 {
	switch {
	case d.name == term:
		return 100
	case strings.HasPrefix(d.name, term):
		return 60
	}
	best := 0.0
	for _, p := range d.nameParts {
		if p == term {
			best = 55
		}
	}
	if best == 0 && len(term) >= 3 && strings.Contains(d.name, term) {
		// Weak: "port" is inside "export" by accident far more often than not.
		best = 15
	}
	if best >= 55 {
		return best
	}
	for _, w := range d.words {
		best = max(best, matchWord(w, term))
		if best >= 30 {
			return best
		}
	}
	if best == 0 && len(term) >= 4 {
		// Typo in the command name itself.
		if editDistance(d.name, term) <= 1 {
			best = 25
		}
	}
	return best
}

func matchWord(w, term string) float64 {
	switch {
	case w == term:
		return 30
	case strings.HasPrefix(w, term):
		// "compress" finds "compression"; very short prefixes are weak.
		if len(term) >= 3 {
			return 22
		}
		return 6
	case len(w) >= 4 && strings.HasPrefix(term, w):
		// "compression" finds "compress"
		return 18
	case len(term) >= 4 && commonStem(w, term) >= max(4, len(term)-3):
		// "deleting" finds "delete", "searches" finds "search"
		return 16
	case len(term) >= 4 && strings.Contains(w, term):
		return 12
	case len(term) >= 4 && len(w) >= 4:
		limit := 1
		if len(term) >= 8 {
			limit = 2
		}
		if editDistance(w, term) <= limit {
			return 8
		}
	}
	return 0
}

func commonStem(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// editDistance is Damerau-Levenshtein (optimal string alignment).
func editDistance(a, b string) int {
	if abs(len(a)-len(b)) > 2 {
		return 3
	}
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// synonyms maps how people describe a task to how man pages tend to.
var synonyms = func() map[string][]string {
	groups := [][]string{
		{"delete", "remove", "erase", "unlink", "rm"},
		{"find", "search", "locate", "lookup", "grep"},
		{"zip", "compress", "archive", "pack", "deflate"},
		{"unzip", "decompress", "extract", "expand", "unpack", "inflate"},
		{"download", "fetch", "retrieve", "transfer", "get"},
		{"upload", "transfer", "send", "put"},
		{"copy", "duplicate", "cp", "clone"},
		{"move", "rename", "mv"},
		{"show", "display", "print", "list", "view"},
		{"edit", "modify", "change", "editor"},
		{"kill", "terminate", "signal", "stop"},
		{"process", "processes", "task", "job", "pid"},
		{"folder", "directory", "dir"},
		{"disk", "volume", "filesystem", "storage", "drive"},
		{"size", "usage", "space"},
		{"network", "net", "interface", "connection"},
		{"internet", "network", "web", "http", "url"},
		{"image", "picture", "photo", "graphics", "png", "jpeg"},
		{"video", "movie", "media"},
		{"audio", "sound", "music"},
		{"password", "credential", "keychain", "secret"},
		{"encrypt", "cipher", "crypto", "gpg"},
		{"hash", "checksum", "digest", "sha", "md5"},
		{"time", "clock", "date"},
		{"schedule", "cron", "periodic", "timer"},
		{"user", "account", "login"},
		{"permission", "permissions", "mode", "chmod", "access"},
		{"owner", "ownership", "chown"},
		{"text", "string", "line", "lines"},
		{"replace", "substitute", "sed"},
		{"count", "wc", "tally"},
		{"sort", "order"},
		{"diff", "compare", "difference", "differences"},
		{"clipboard", "pasteboard", "paste"},
		{"monitor", "watch", "top", "activity"},
		{"memory", "ram", "mem"},
		{"cpu", "processor"},
		{"battery", "power"},
		{"screenshot", "screencapture", "capture"},
		{"speak", "say", "speech", "tts"},
		{"json", "jq"},
		{"wifi", "airport", "wireless"},
		{"dns", "domain", "resolve", "nameserver"},
		{"ssh", "remote", "secure", "shell"},
		{"open", "launch", "start"},
		{"tree", "hierarchy", "recursive", "recursively"},
		{"tarball", "tar", "archive", "tgz"},
		{"port", "socket", "listen", "listening"},
		{"resize", "scale", "resample", "dimensions"},
	}
	m := map[string][]string{}
	for _, g := range groups {
		for _, w := range g {
			for _, other := range g {
				if other != w {
					m[w] = append(m[w], other)
				}
			}
		}
	}
	return m
}()
