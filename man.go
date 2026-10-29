package main

import (
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

type manInfo struct {
	desc    string
	section string
}

// Sections that document things you can run. Earlier is preferred when a
// name appears in several.
var manSections = []string{"1", "8", "6"}

// manPath returns every directory that might hold man pages: what man(1)
// itself would search, plus the conventional man dir beside each PATH entry
// (so tools whose docs man doesn't know about still get described).
func manPath() []string {
	var cands []string
	if out, err := exec.Command("manpath").Output(); err == nil {
		cands = filepath.SplitList(strings.TrimSpace(string(out)))
	} else {
		cands = append(filepath.SplitList(os.Getenv("MANPATH")), "/usr/share/man", "/usr/local/share/man")
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		cands = append(cands, filepath.Join(dir, "..", "share", "man"), filepath.Join(dir, "..", "man"))
	}
	var roots []string
	seen := map[string]bool{}
	for _, c := range cands {
		real, err := filepath.EvalSymlinks(c)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		roots = append(roots, c)
	}
	return roots
}

// scanMan parses the NAME section of every section 1/8/6 man page and
// returns command name -> summary.
func scanMan() map[string]manInfo {
	type job struct{ root, file, section string }
	jobs := make(chan job)
	type result struct {
		names   []string
		desc    string
		section string
	}
	results := make(chan result)

	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				names, desc := parseManFile(j.root, j.file, 0)
				if desc == "" {
					continue
				}
				// The file name is a name too, even if NAME omits it.
				base := strings.TrimSuffix(filepath.Base(j.file), ".gz")
				if i := strings.LastIndex(base, "."); i > 0 {
					names = append(names, base[:i])
				}
				results <- result{names, desc, j.section}
			}
		}()
	}

	go func() {
		seen := map[string]bool{}
		for _, root := range manPath() {
			for _, sect := range manSections {
				dir := filepath.Join(root, "man"+sect)
				ents, err := os.ReadDir(dir)
				if err != nil {
					continue
				}
				for _, de := range ents {
					f := filepath.Join(dir, de.Name())
					if real, err := filepath.EvalSymlinks(f); err == nil {
						if seen[real] {
							continue
						}
						seen[real] = true
					}
					jobs <- job{root, f, sect}
				}
			}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	rank := func(s string) int {
		for i, x := range manSections {
			if x == s {
				return i
			}
		}
		return len(manSections)
	}
	out := map[string]manInfo{}
	for r := range results {
		for _, n := range r.names {
			if old, ok := out[n]; ok && rank(old.section) <= rank(r.section) {
				continue
			}
			out[n] = manInfo{r.desc, r.section}
		}
	}
	return out
}

func openMan(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, ".gz") {
		return f, nil
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return struct {
		io.Reader
		io.Closer
	}{gz, f}, nil
}

// parseManFile extracts the names and one-line description from a man
// page's NAME section. It understands both man(7) (".SH NAME" followed by
// "foo, bar \- desc") and mdoc(7) (".Nm foo" / ".Nd desc"), and follows
// ".so" redirects.
func parseManFile(root, path string, depth int) (names []string, desc string) {
	rc, err := openMan(path)
	if err != nil {
		return nil, ""
	}
	defer rc.Close()

	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	inName := false
	var text []string
	for lines := 0; sc.Scan() && lines < 400; lines++ {
		line := sc.Text()
		if lines < 5 && strings.HasPrefix(line, ".so ") && depth < 3 {
			target := strings.TrimSpace(line[4:])
			if !filepath.IsAbs(target) {
				target = filepath.Join(root, target)
			}
			if _, err := os.Stat(target); err != nil {
				target += ".gz"
			}
			return parseManFile(root, target, depth+1)
		}
		macro, args := splitMacro(line)
		switch macro {
		case ".SH", ".Sh", ".SS":
			if inName {
				goto done
			}
			inName = strings.EqualFold(strings.Trim(args, `" `), "NAME")
			continue
		}
		if !inName {
			continue
		}
		switch macro {
		case ".Nm":
			for _, n := range strings.Fields(args) {
				if n = strings.Trim(n, ","); n != "" {
					names = append(names, n)
				}
			}
		case ".Nd":
			desc = args
		case "":
			text = append(text, line)
		case ".B", ".I", ".BR", ".BI", ".IR", ".RB", ".RI", ".IB", ".Ar", ".Op":
			text = append(text, strings.ReplaceAll(args, `"`, ""))
		}
	}
done:
	if desc != "" {
		return names, cleanRoff(desc)
	}
	// man(7): "name1, name2 \- description"
	joined := cleanRoff(strings.Join(text, " "))
	for _, sep := range []string{" - ", " -- ", " — ", " – "} {
		if i := strings.Index(joined, sep); i > 0 {
			for _, n := range strings.Split(joined[:i], ",") {
				if n = strings.TrimSpace(n); n != "" && !strings.Contains(n, " ") {
					names = append(names, n)
				}
			}
			return names, strings.TrimSpace(joined[i+len(sep):])
		}
	}
	return names, ""
}

func splitMacro(line string) (macro, args string) {
	if !strings.HasPrefix(line, ".") && !strings.HasPrefix(line, "'") {
		return "", line
	}
	if strings.HasPrefix(line, `.\"`) || strings.HasPrefix(line, `'\"`) {
		return ".\\\"", ""
	}
	macro, args, _ = strings.Cut(line, " ")
	return macro, strings.TrimSpace(args)
}

var (
	roffFont    = regexp.MustCompile(`\\f(\[[^\]]*\]|\(..|.)`)
	roffSize    = regexp.MustCompile(`\\s[+-]?\d`)
	roffSpecial = regexp.MustCompile(`\\(\(..|\[[^\]]*\])`)
	roffOther   = regexp.MustCompile(`\\[&%|^:,/{}]`)
	spaces      = regexp.MustCompile(`\s+`)
)

var roffChars = map[string]string{
	`\(em`: "—", `\(en`: "–", `\(hy`: "-", `\(aq`: "'", `\(dq`: `"`,
	`\(lq`: `"`, `\(rq`: `"`, `\(oq`: "'", `\(cq`: "'", `\(bu`: "•",
	`\[em]`: "—", `\[en]`: "–", `\[aq]`: "'", `\[dq]`: `"`,
}

func cleanRoff(s string) string {
	s = strings.ReplaceAll(s, `\-`, "-")
	s = strings.ReplaceAll(s, `\e`, `\`)
	s = strings.ReplaceAll(s, `\ `, " ")
	s = strings.ReplaceAll(s, `\(rs`, "\x00")
	s = roffFont.ReplaceAllString(s, "")
	s = roffSize.ReplaceAllString(s, "")
	s = roffSpecial.ReplaceAllStringFunc(s, func(m string) string { return roffChars[m] })
	s = roffOther.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\x00", `\`)
	s = strings.Trim(s, `" `)
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}
