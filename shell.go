package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
