package ruby

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseName(t *testing.T) {
	cases := []struct {
		name            string
		engine, version string
		ok              bool
	}{
		{"ruby-4.0.5", "ruby", "4.0.5", true},
		{"ruby-3.4.5", "ruby", "3.4.5", true},
		{"jruby-9.4.0.0", "jruby", "9.4.0.0", true},
		{"truffleruby-24.1.0", "truffleruby", "24.1.0", true},
		{"ruby-3.4.0-preview1", "ruby", "3.4.0-preview1", true},
		{"3.4.5", "ruby", "3.4.5", true},
		{"my-custom-ruby", "", "", false},
		{"system", "", "", false},
	}
	for _, c := range cases {
		e, v, ok := parseName(c.name)
		if ok != c.ok || e != c.engine || v != c.version {
			t.Errorf("parseName(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.name, e, v, ok, c.engine, c.version, c.ok)
		}
	}
}

func TestMatch(t *testing.T) {
	rubies := []Ruby{
		{Root: "/r/ruby-3.4.5"}, {Root: "/r/ruby-4.0.2"}, {Root: "/r/ruby-4.0.5"},
	}
	cases := []struct {
		query string
		want  string
		ok    bool
	}{
		{"ruby-4.0.5", "ruby-4.0.5", true}, // exact basename wins
		{"3.4.5", "ruby-3.4.5", true},
		{"4.0", "ruby-4.0.5", true}, // last substring match
		{"nope", "", false},
	}
	for _, c := range cases {
		m, ok := Match(rubies, c.query)
		if ok != c.ok || (ok && m.Name() != c.want) {
			t.Errorf("Match(%q) = (%q,%v), want (%q,%v)", c.query, m.Name(), ok, c.want, c.ok)
		}
	}
}

// stmtMap folds a statement slice into a name->value map plus a set of unsets,
// for order-independent assertions.
func stmtMap(stmts []Stmt) (map[string]string, map[string]bool) {
	set := map[string]string{}
	uns := map[string]bool{}
	for _, s := range stmts {
		if s.Unset {
			uns[s.Name] = true
			delete(set, s.Name)
		} else {
			set[s.Name] = s.Value
			delete(uns, s.Name)
		}
	}
	return set, uns
}

func TestUseNonRoot(t *testing.T) {
	r := Ruby{Root: "/opt/rubies/ruby-4.0.5", Engine: "ruby", Version: "4.0.5"}
	env := NewEnv(map[string]string{"PATH": "/usr/bin:/bin"}, 501, "/home/x")

	set, _ := stmtMap(env.Use(r, ""))

	want := map[string]string{
		"RUBY_ROOT":    "/opt/rubies/ruby-4.0.5",
		"RUBY_ENGINE":  "ruby",
		"RUBY_VERSION": "4.0.5",
		"GEM_HOME":     "/home/x/.gem/ruby/4.0.5",
	}
	for k, v := range want {
		if set[k] != v {
			t.Errorf("%s = %q, want %q", k, set[k], v)
		}
	}
	// No GEM_ROOT on disk in this fixture, so GEM_PATH is just GEM_HOME.
	if set["GEM_PATH"] != "/home/x/.gem/ruby/4.0.5" {
		t.Errorf("GEM_PATH = %q", set["GEM_PATH"])
	}
	if got := set["PATH"]; !strings.HasPrefix(got, "/home/x/.gem/ruby/4.0.5/bin:/opt/rubies/ruby-4.0.5/bin:") {
		t.Errorf("PATH prefix wrong: %q", got)
	}
	if _, ok := set["RUBYOPT"]; ok {
		t.Error("RUBYOPT should be absent when no opts passed")
	}
}

func TestUseRootUID(t *testing.T) {
	r := Ruby{Root: "/opt/rubies/ruby-4.0.5", Engine: "ruby", Version: "4.0.5"}
	env := NewEnv(map[string]string{"PATH": "/usr/bin"}, 0, "/root")

	set, _ := stmtMap(env.Use(r, ""))

	for _, k := range []string{"GEM_HOME", "GEM_PATH"} {
		if _, ok := set[k]; ok {
			t.Errorf("root UID must not set %s", k)
		}
	}
	if set["PATH"] != "/opt/rubies/ruby-4.0.5/bin:/usr/bin" {
		t.Errorf("PATH = %q", set["PATH"])
	}
}

func TestUseWithRubyOpt(t *testing.T) {
	r := Ruby{Root: "/r/ruby-4.0.5", Engine: "ruby", Version: "4.0.5"}
	env := NewEnv(map[string]string{"PATH": "/usr/bin"}, 501, "/home/x")
	set, _ := stmtMap(env.Use(r, "-W2"))
	if set["RUBYOPT"] != "-W2" {
		t.Errorf("RUBYOPT = %q, want -W2", set["RUBYOPT"])
	}
}

func TestUseResetsPrevious(t *testing.T) {
	// A ruby is already active; switching must strip the old bin dirs from PATH.
	env := NewEnv(map[string]string{
		"PATH":         "/home/x/.gem/ruby/3.4.5/bin:/r/ruby-3.4.5/bin:/usr/bin",
		"RUBY_ROOT":    "/r/ruby-3.4.5",
		"RUBY_VERSION": "3.4.5",
		"RUBY_ENGINE":  "ruby",
		"GEM_HOME":     "/home/x/.gem/ruby/3.4.5",
		"GEM_PATH":     "/home/x/.gem/ruby/3.4.5",
	}, 501, "/home/x")

	r := Ruby{Root: "/r/ruby-4.0.5", Engine: "ruby", Version: "4.0.5"}
	set, _ := stmtMap(env.Use(r, ""))

	path := set["PATH"]
	if strings.Contains(path, "ruby-3.4.5") {
		t.Errorf("old ruby still on PATH: %q", path)
	}
	if !strings.HasPrefix(path, "/home/x/.gem/ruby/4.0.5/bin:/r/ruby-4.0.5/bin:") {
		t.Errorf("new ruby not at front: %q", path)
	}
	if !strings.HasSuffix(path, ":/usr/bin") {
		t.Errorf("base PATH lost: %q", path)
	}
}

func TestReset(t *testing.T) {
	env := NewEnv(map[string]string{
		"PATH":         "/home/x/.gem/ruby/4.0.5/bin:/r/ruby-4.0.5/bin:/usr/bin",
		"RUBY_ROOT":    "/r/ruby-4.0.5",
		"RUBY_VERSION": "4.0.5",
		"RUBY_ENGINE":  "ruby",
		"GEM_HOME":     "/home/x/.gem/ruby/4.0.5",
		"GEM_PATH":     "/home/x/.gem/ruby/4.0.5",
	}, 501, "/home/x")

	set, uns := stmtMap(env.Reset())

	if set["PATH"] != "/usr/bin" {
		t.Errorf("PATH = %q, want /usr/bin", set["PATH"])
	}
	for _, k := range []string{"RUBY_ROOT", "RUBY_ENGINE", "RUBY_VERSION", "RUBYOPT", "GEM_ROOT", "GEM_HOME", "GEM_PATH"} {
		if !uns[k] {
			t.Errorf("%s should be unset", k)
		}
	}
}

func TestResetNoActiveRuby(t *testing.T) {
	env := NewEnv(map[string]string{"PATH": "/usr/bin"}, 501, "/home/x")
	if got := env.Reset(); got != nil {
		t.Errorf("reset with no active ruby should be a no-op, got %v", got)
	}
}

func TestGemRootGlob(t *testing.T) {
	root := t.TempDir()
	gemsDir := filepath.Join(root, "lib", "ruby", "gems", "4.0.0")
	if err := os.MkdirAll(gemsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gemRoot(root); got != gemsDir {
		t.Errorf("gemRoot = %q, want %q", got, gemsDir)
	}

	env := NewEnv(map[string]string{"PATH": "/usr/bin"}, 501, "/home/x")
	r := Ruby{Root: root, Engine: "ruby", Version: "4.0.5"}
	set, _ := stmtMap(env.Use(r, ""))
	if set["GEM_ROOT"] != gemsDir {
		t.Errorf("GEM_ROOT = %q, want %q", set["GEM_ROOT"], gemsDir)
	}
	if want := gemsDir + "/bin"; !strings.Contains(set["PATH"], want) {
		t.Errorf("PATH missing gem root bin %q: %q", want, set["PATH"])
	}
}

func TestRenderQuoting(t *testing.T) {
	out := Render([]Stmt{
		set("RUBY_ROOT", "/r/ruby-4.0.5"),
		set("WEIRD", "a b'c"),
		unset("GEM_HOME"),
	})
	wantLines := []string{
		`export RUBY_ROOT='/r/ruby-4.0.5'`,
		`export WEIRD='a b'\''c'`,
		`unset GEM_HOME`,
	}
	for _, l := range wantLines {
		if !strings.Contains(out, l) {
			t.Errorf("Render missing line %q in:\n%s", l, out)
		}
	}
}
