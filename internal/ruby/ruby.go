// Package ruby computes the environment changes for switching Ruby versions,
// mirroring chruby's contract without ever spawning ruby.
package ruby

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Env is an injectable view of the process environment, so the switch logic can
// be unit-tested without touching os.Environ.
type Env struct {
	vars map[string]string
	uid  int
	home string
}

// EnvFromOS snapshots the real process environment.
func EnvFromOS() *Env {
	vars := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			vars[k] = v
		}
	}
	return &Env{vars: vars, uid: os.Getuid(), home: os.Getenv("HOME")}
}

// NewEnv builds an Env from an explicit map (test seam).
func NewEnv(vars map[string]string, uid int, home string) *Env {
	if vars == nil {
		vars = map[string]string{}
	}
	return &Env{vars: vars, uid: uid, home: home}
}

func (e *Env) get(k string) string { return e.vars[k] }

// Stmt is one shell statement to be eval'd by the caller's shell.
type Stmt struct {
	Unset bool
	Name  string
	Value string // export value (raw, unquoted); ignored when Unset
}

// A Ruby is an installed interpreter discovered under a search root.
type Ruby struct {
	Root    string // e.g. /Users/x/.rubies/ruby-4.0.5
	Engine  string // ruby, jruby, truffleruby, …
	Version string // 4.0.5
}

// Name is the directory basename, chruby's identity for a ruby.
func (r Ruby) Name() string { return filepath.Base(r.Root) }

var nameRE = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_+-]*?)-([0-9][0-9A-Za-z.+_-]*)$`)

// parseName splits a chruby-style dir name into engine + version. A bare
// numeric name ("3.4.5") is treated as engine "ruby". Returns ok=false when the
// name has no recognisable version component.
func parseName(name string) (engine, version string, ok bool) {
	if m := nameRE.FindStringSubmatch(name); m != nil {
		return m[1], m[2], true
	}
	if regexp.MustCompile(`^[0-9]`).MatchString(name) {
		return "ruby", name, true
	}
	return "", "", false
}

// NewRuby builds a Ruby from an install directory, parsing engine/version from
// its basename.
func NewRuby(root string) (Ruby, bool) {
	engine, version, ok := parseName(filepath.Base(root))
	if !ok {
		return Ruby{}, false
	}
	return Ruby{Root: root, Engine: engine, Version: version}, true
}

// SearchDirs returns chruby's ruby search roots, honouring $PREFIX and $HOME.
func SearchDirs(prefix, home string) []string {
	var dirs []string
	if prefix != "" {
		dirs = append(dirs, filepath.Join(prefix, "opt", "rubies"))
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".rubies"))
	}
	return dirs
}

// Discover lists rubies found under the given search dirs, sorted by name.
func Discover(searchDirs []string) []Ruby {
	var out []Ruby
	seen := map[string]bool{}
	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			root := filepath.Join(dir, e.Name())
			if !isRubyRoot(root) || seen[root] {
				continue
			}
			if r, ok := NewRuby(root); ok {
				seen[root] = true
				out = append(out, r)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func isRubyRoot(root string) bool {
	info, err := os.Stat(filepath.Join(root, "bin", "ruby"))
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// Match resolves a query against the installed rubies using chruby's rules: an
// exact basename match wins immediately; otherwise the last substring match is
// used.
func Match(rubies []Ruby, query string) (Ruby, bool) {
	var match Ruby
	var found bool
	for _, r := range rubies {
		if r.Name() == query {
			return r, true
		}
		if strings.Contains(r.Name(), query) {
			match, found = r, true
		}
	}
	return match, found
}

// gemRoot returns the interpreter's default gem dir (Gem.default_dir) by glob,
// the deterministic equivalent of chruby's `ruby -e 'puts Gem.default_dir'`.
func gemRoot(root string) string {
	matches, _ := filepath.Glob(filepath.Join(root, "lib", "ruby", "gems", "*"))
	sort.Strings(matches)
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.IsDir() {
			return m
		}
	}
	return ""
}

// pathList splits/joins a colon-delimited PATH-like variable.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ":")
}

func without(list []string, drop ...string) []string {
	dropped := map[string]bool{}
	for _, d := range drop {
		if d != "" {
			dropped[d] = true
		}
	}
	out := list[:0:0]
	for _, item := range list {
		if !dropped[item] {
			out = append(out, item)
		}
	}
	return out
}

// Use produces the statements to activate the ruby at root, first resetting any
// currently-active ruby (as chruby_use does).
func (e *Env) Use(r Ruby, rubyOpt string) []Stmt {
	var stmts []Stmt
	if e.get("RUBY_ROOT") != "" {
		stmts = append(stmts, e.reset()...)
		e = e.applied(stmts) // fold the reset in so PATH math below is correct
	}

	gemRootDir := gemRoot(r.Root)
	nonRoot := e.uid != 0

	stmts = append(stmts,
		set("RUBY_ROOT", r.Root),
		set("RUBY_ENGINE", r.Engine),
		set("RUBY_VERSION", r.Version),
	)
	if rubyOpt != "" {
		stmts = append(stmts, set("RUBYOPT", rubyOpt))
	}
	if gemRootDir != "" {
		stmts = append(stmts, set("GEM_ROOT", gemRootDir))
	}

	pathAdd := []string{filepath.Join(r.Root, "bin")}
	if nonRoot {
		gemHome := filepath.Join(e.home, ".gem", r.Engine, r.Version)
		gemPath := gemHome
		if gemRootDir != "" {
			gemPath += ":" + gemRootDir
		}
		if existing := e.get("GEM_PATH"); existing != "" {
			gemPath += ":" + existing
		}
		stmts = append(stmts, set("GEM_HOME", gemHome), set("GEM_PATH", gemPath))
		pathAdd = []string{filepath.Join(gemHome, "bin")}
		if gemRootDir != "" {
			pathAdd = append(pathAdd, filepath.Join(gemRootDir, "bin"))
		}
		pathAdd = append(pathAdd, filepath.Join(r.Root, "bin"))
	}

	newPath := append(append([]string{}, pathAdd...), splitList(e.get("PATH"))...)
	stmts = append(stmts, set("PATH", strings.Join(newPath, ":")))
	return stmts
}

// Reset produces the statements to return to system ruby.
func (e *Env) Reset() []Stmt { return e.reset() }

func (e *Env) reset() []Stmt {
	rubyRoot := e.get("RUBY_ROOT")
	if rubyRoot == "" {
		return nil
	}
	gemHome := e.get("GEM_HOME")
	gemRootDir := e.get("GEM_ROOT")

	path := splitList(e.get("PATH"))
	if e.uid != 0 {
		path = without(path, filepath.Join(rubyRoot, "bin"),
			binOf(gemHome), binOf(gemRootDir))
	} else {
		path = without(path, filepath.Join(rubyRoot, "bin"))
	}
	stmts := []Stmt{set("PATH", strings.Join(path, ":"))}

	if e.uid != 0 {
		gemPath := without(splitList(e.get("GEM_PATH")), gemHome, gemRootDir)
		if len(gemPath) == 0 {
			stmts = append(stmts, unset("GEM_PATH"))
		} else {
			stmts = append(stmts, set("GEM_PATH", strings.Join(gemPath, ":")))
		}
		stmts = append(stmts, unset("GEM_ROOT"), unset("GEM_HOME"))
	}
	stmts = append(stmts, unset("RUBY_ROOT"), unset("RUBY_ENGINE"),
		unset("RUBY_VERSION"), unset("RUBYOPT"))
	return stmts
}

func binOf(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "bin")
}

// applied returns a copy of e with the given statements folded into its vars, so
// successive computations (reset then use) see a consistent view.
func (e *Env) applied(stmts []Stmt) *Env {
	vars := maps.Clone(e.vars)
	for _, s := range stmts {
		if s.Unset {
			delete(vars, s.Name)
		} else {
			vars[s.Name] = s.Value
		}
	}
	return &Env{vars: vars, uid: e.uid, home: e.home}
}

func set(name, value string) Stmt { return Stmt{Name: name, Value: value} }
func unset(name string) Stmt      { return Stmt{Unset: true, Name: name} }

// Render formats statements as POSIX shell, safely single-quoted.
func Render(stmts []Stmt) string {
	var b strings.Builder
	for _, s := range stmts {
		if s.Unset {
			fmt.Fprintf(&b, "unset %s\n", s.Name)
		} else {
			fmt.Fprintf(&b, "export %s=%s\n", s.Name, shellQuote(s.Value))
		}
	}
	return b.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
