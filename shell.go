package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// brewDescs returns formula/cask name -> description for everything
// installed via Homebrew.
func brewDescs() map[string]string {
	out := map[string]string{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "brew", "info", "--json=v2", "--installed").Output()
	if err != nil {
		return out
	}
	var info struct {
		Formulae []struct{ Name, Desc string }
		Casks    []struct{ Token, Desc string }
	}
	if json.Unmarshal(raw, &info) != nil {
		return out
	}
	for _, f := range info.Formulae {
		out[f.Name] = f.Desc
	}
	for _, c := range info.Casks {
		out[c.Token] = c.Desc
	}
	return out
}

// brewOwner returns the formula or cask that provides the binary at path,
// by resolving the symlink Homebrew put on PATH back into the Cellar or
// Caskroom.
func brewOwner(path string) string {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	for _, marker := range []string{"/Cellar/", "/Caskroom/"} {
		if _, rest, ok := strings.Cut(real, marker); ok {
			pkg, _, _ := strings.Cut(rest, "/")
			return pkg
		}
	}
	return ""
}

func userShell() string {
	sh := filepath.Base(os.Getenv("SHELL"))
	if sh == "" || sh == "." {
		return "zsh"
	}
	return sh
}

// shellBuiltins lists the user's shell builtins and reserved words. The
// shell is started without rc files, so this is fast and side-effect free.
func shellBuiltins() []string {
	var cmd *exec.Cmd
	switch userShell() {
	case "zsh":
		cmd = exec.Command("zsh", "-f", "-c", `print -l ${(k)builtins} ${(k)reswords}`)
	case "bash":
		cmd = exec.Command("bash", "--norc", "--noprofile", "-c", "compgen -b; compgen -k")
	case "fish":
		cmd = exec.Command("fish", "--no-config", "-c", "builtin -n")
	default:
		return nil
	}
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

// shellAliases asks an interactive instance of the user's shell for its
// aliases. Interactive startup can be slow or noisy, so this is best effort
// with a timeout.
func shellAliases() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch sh := userShell(); sh {
	case "zsh", "bash":
		cmd = exec.CommandContext(ctx, sh, "-i", "-c", "alias")
	case "fish":
		cmd = exec.CommandContext(ctx, "fish", "-i", "-c", "alias")
	default:
		return nil
	}
	cmd.Stdin = nil
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil
	}
	aliases := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimPrefix(line, "alias ")
		name, val, ok := strings.Cut(line, "=")
		if !ok {
			// fish: "alias name 'value'"
			name, val, ok = strings.Cut(line, " ")
			if !ok {
				continue
			}
		}
		name = strings.Trim(name, `'"`)
		if name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		aliases[name] = "alias for: " + strings.Trim(val, `'"`)
	}
	return aliases
}

// funcInfo is a shell function: where it was defined and what it does.
type funcInfo struct {
	src  string
	desc string
}

// shellFunctions asks an interactive instance of the user's shell for its
// functions and the file each was defined in. Descriptions come from fish's
// own --description, the README next to the source (Oh My Zsh plugins keep a
// table of their commands there), or the comment just above the definition.
// Underscore-prefixed names, families of helpers like nvm_*, and
// undocumented functions from outside the home directory (the shell's own
// plumbing, like compinit) are skipped.
func shellFunctions() map[string]funcInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch userShell() {
	case "zsh":
		cmd = exec.CommandContext(ctx, "zsh", "-i", "-c",
			`for f in ${(k)functions}; do print -r -- "$f"$'\t'"${functions_source[$f]}"; done`)
	case "bash":
		cmd = exec.CommandContext(ctx, "bash", "-i", "-c",
			`shopt -s extdebug; for f in $(compgen -A function); do declare -F "$f" | { read -r n l s; printf '%s\t%s\n' "$n" "$s"; }; done`)
	case "fish":
		cmd = exec.CommandContext(ctx, "fish", "-i", "-c",
			`for f in (functions -n); set d (functions -Dv $f); printf '%s\t%s\t%s\n' $f $d[1] $d[5]; end`)
	default:
		return nil
	}
	cmd.Stdin = nil
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil
	}

	funcs := map[string]funcInfo{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\t")
		name := f[0]
		if name == "" || strings.ContainsAny(name, " /:") || strings.ContainsRune("_+.-", rune(name[0])) {
			continue
		}
		fi := funcInfo{}
		if len(f) > 1 && filepath.IsAbs(f[1]) {
			fi.src = f[1]
		}
		if len(f) > 2 && f[2] != "n/a" {
			fi.desc = f[2]
		}
		funcs[name] = fi
	}

	// Plumbing tends to come in families sharing a prefix or suffix
	// (nvm_*, omz_*, *_prompt_info); real commands rarely do.
	family := map[string]int{}
	for name := range funcs {
		if first, last, ok := affixes(name); ok {
			family["<"+first]++
			family[">"+last]++
		}
	}
	home, _ := os.UserHomeDir()
	docs := docCache{}
	for name, fi := range funcs {
		if first, last, ok := affixes(name); ok && (family["<"+first] >= 3 || family[">"+last] >= 3) {
			delete(funcs, name)
			continue
		}
		if fi.desc == "" && fi.src != "" {
			fi.desc = docs.readme(filepath.Dir(fi.src))[name]
			if fi.desc == "" && home != "" && strings.HasPrefix(fi.src, home+"/") {
				fi.desc = docs.comment(fi.src, name)
			}
		}
		if fi.desc == "" && (home == "" || !strings.HasPrefix(fi.src, home+"/")) {
			delete(funcs, name)
			continue
		}
		funcs[name] = fi
	}
	return funcs
}

// affixes splits a name like "git_prompt_info" into its first and last
// segments.
func affixes(name string) (first, last string, ok bool) {
	i := strings.IndexAny(name, "_-")
	j := strings.LastIndexAny(name, "_-")
	if i < 0 {
		return "", "", false
	}
	return name[:i], name[j+1:], true
}

// docCache reads each README and source file at most once.
type docCache struct {
	readmes map[string]map[string]string
	sources map[string][]string
}

var (
	readmeRow  = regexp.MustCompile("^\\|\\s*`([^`\\s]+)[^|]*\\|\\s*([^|]+?)\\s*\\|")
	codeOnly   = regexp.MustCompile("^`[^`]*`$")
	commentPfx = regexp.MustCompile(`^\s*#+\s?`)
)

// readme returns name -> description from the markdown tables in dir's
// README, skipping rows whose description is just the code it runs (or a
// fragment of it, when the code contains a "|").
func (c *docCache) readme(dir string) map[string]string {
	if m, ok := c.readmes[dir]; ok {
		return m
	}
	if c.readmes == nil {
		c.readmes = map[string]map[string]string{}
	}
	m := map[string]string{}
	if raw, err := os.ReadFile(filepath.Join(dir, "README.md")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if r := readmeRow.FindStringSubmatch(line); r != nil && !codeOnly.MatchString(r[2]) && strings.Count(r[2], "`")%2 == 0 {
				if _, dup := m[r[1]]; !dup {
					m[r[1]] = r[2]
				}
			}
		}
	}
	c.readmes[dir] = m
	return m
}

// comment returns the first paragraph of the comment block directly above
// name's definition in the file at path.
func (c *docCache) comment(path, name string) string {
	lines, ok := c.sources[path]
	if !ok {
		if c.sources == nil {
			c.sources = map[string][]string{}
		}
		if raw, err := os.ReadFile(path); err == nil {
			lines = strings.Split(string(raw), "\n")
		}
		c.sources[path] = lines
	}
	q := regexp.QuoteMeta(name)
	def := regexp.MustCompile(`^\s*(function\s+` + q + `\b|` + q + `\s*\(\s*\))`)
	for i, line := range lines {
		if !def.MatchString(line) {
			continue
		}
		start := i
		for start > 0 && commentPfx.MatchString(lines[start-1]) && !strings.HasPrefix(lines[start-1], "#!") {
			start--
		}
		var para []string
		for _, l := range lines[start:i] {
			text := strings.TrimSpace(commentPfx.ReplaceAllString(l, ""))
			if text == "" || strings.HasPrefix(text, "shellcheck") {
				if len(para) > 0 {
					break
				}
				continue
			}
			para = append(para, text)
		}
		return strings.Join(para, " ")
	}
	return ""
}
