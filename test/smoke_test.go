// Package smoke exercises the built rpup binary through real zsh and bash,
// covering both non-interactive (one-shot default) and interactive (auto-switch
// on cd) usage, and differentially against chruby when it is installed.
package smoke

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var rpupBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rpup-bin")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	rpupBin = filepath.Join(dir, "rpup")
	build := exec.Command("go", "build", "-o", rpupBin, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		panic("build rpup: " + err.Error() + "\n" + string(out))
	}
	os.Exit(m.Run())
}

// fakeHome builds a HOME with two stub rubies and a home-default .ruby-version.
// bin/ruby is a stub that is never executed (rpup is fork-free) but must exist
// and be executable for discovery.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, v := range []string{"ruby-3.4.5", "ruby-4.0.5"} {
		root := filepath.Join(home, ".rubies", v)
		mustMkdir(t, filepath.Join(root, "bin"))
		mustMkdir(t, filepath.Join(root, "lib", "ruby", "gems", strings.TrimPrefix(v, "ruby-")[:3]+".0"))
		mustWrite(t, filepath.Join(root, "bin", "ruby"), "#!/bin/sh\nexit 0\n", 0o755)
	}
	mustWrite(t, filepath.Join(home, ".ruby-version"), "ruby-4.0.5\n", 0o644)

	proj := filepath.Join(home, "proj")
	mustMkdir(t, proj)
	mustWrite(t, filepath.Join(proj, ".ruby-version"), "ruby-3.4.5\n", 0o644)
	return home
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// runShell runs a script under the given shell with HOME set and rpup on PATH,
// returning combined output. The shell starts with -f (no rc) for hermeticity.
func runShell(t *testing.T, shell, home, script string) string {
	t.Helper()
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("%s not installed", shell)
	}
	cmd := exec.Command(shell, "-f", "-c", script)
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + filepath.Dir(rpupBin) + ":/usr/bin:/bin",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s script failed: %v\n%s", shell, err, out)
	}
	return string(out)
}

// TestNonInteractiveDefault: a non-interactive shell that sources the hook lands
// on the home-default ruby (4.0.5) with its bin on PATH — the env.zsh else-branch
// scenario, now served by the same hook.
func TestNonInteractiveDefault(t *testing.T) {
	home := fakeHome(t)
	for _, shell := range []string{"zsh", "bash"} {
		t.Run(shell, func(t *testing.T) {
			script := `eval "$(rpup hook ` + shell + `)"; echo "V=$RUBY_VERSION"; echo "R=$RUBY_ROOT"; case ":$PATH:" in *":$RUBY_ROOT/bin:"*) echo "ONPATH=yes";; *) echo "ONPATH=no";; esac`
			out := runShell(t, shell, home, script)
			assertLine(t, out, "V=4.0.5")
			assertLine(t, out, "R="+filepath.Join(home, ".rubies", "ruby-4.0.5"))
			assertLine(t, out, "ONPATH=yes")
		})
	}
}

// TestInteractiveAutoSwitchZsh: with the hook wired, cd-ing into a project dir
// with a .ruby-version switches ruby, and leaving it restores the home default.
// zsh chpwd hooks fire on cd even under -c, so this exercises the interactive path.
func TestInteractiveAutoSwitchZsh(t *testing.T) {
	home := fakeHome(t)
	script := `eval "$(rpup hook zsh)"
echo "start=$RUBY_VERSION"
cd "$HOME/proj"; echo "proj=$RUBY_VERSION"
cd "$HOME"; echo "home=$RUBY_VERSION"`
	out := runShell(t, "zsh", home, script)
	assertLine(t, out, "start=4.0.5")
	assertLine(t, out, "proj=3.4.5")
	assertLine(t, out, "home=4.0.5")
}

// TestInteractiveAutoSwitchBash: bash wires _rpup_auto via PROMPT_COMMAND, which
// does not fire under -c, so we invoke it explicitly after each cd — the same
// call the prompt would make interactively.
func TestInteractiveAutoSwitchBash(t *testing.T) {
	home := fakeHome(t)
	script := `eval "$(rpup hook bash)"
echo "start=$RUBY_VERSION"
cd "$HOME/proj"; _rpup_auto; echo "proj=$RUBY_VERSION"
cd "$HOME"; _rpup_auto; echo "home=$RUBY_VERSION"`
	out := runShell(t, "bash", home, script)
	assertLine(t, out, "start=4.0.5")
	assertLine(t, out, "proj=3.4.5")
	assertLine(t, out, "home=4.0.5")
}

// TestResetToSystem: `rpup reset` clears the ruby env cleanly.
func TestResetToSystem(t *testing.T) {
	home := fakeHome(t)
	script := `eval "$(rpup hook zsh)"; rpup reset; echo "R=[$RUBY_ROOT]"; echo "V=[$RUBY_VERSION]"`
	out := runShell(t, "zsh", home, script)
	assertLine(t, out, "R=[]")
	assertLine(t, out, "V=[]")
}

// TestDifferentialAgainstChruby switches to each real installed ruby under both
// chruby and rpup and asserts the RUBY_*/GEM_* environment is identical — the
// drop-in guarantee. Skips unless chruby and real rubies are present.
func TestDifferentialAgainstChruby(t *testing.T) {
	chrubySh := "/opt/homebrew/opt/chruby/share/chruby/chruby.sh"
	if _, err := os.Stat(chrubySh); err != nil {
		t.Skip("chruby not installed")
	}
	realHome, _ := os.UserHomeDir()
	rubies, err := os.ReadDir(filepath.Join(realHome, ".rubies"))
	if err != nil || len(rubies) == 0 {
		t.Skip("no real rubies under ~/.rubies")
	}
	target := rubies[len(rubies)-1].Name()

	vars := []string{"RUBY_ROOT", "RUBY_ENGINE", "RUBY_VERSION", "GEM_ROOT", "GEM_HOME", "GEM_PATH"}
	printVars := ""
	for _, name := range vars {
		printVars += "echo " + name + "=\"$" + name + "\";"
	}

	chrubyScript := "source '" + chrubySh + "'; chruby '" + target + "' >/dev/null 2>&1;" + printVars
	rpupScript := "eval \"$(rpup hook zsh)\"; rpup use '" + target + "';" + printVars

	envFor := func(script string) map[string]string {
		cmd := exec.Command("zsh", "-f", "-c", script)
		cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(rpupBin)+":"+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("script failed: %v\n%s", err, out)
		}
		m := map[string]string{}
		for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
			if k, v, ok := strings.Cut(line, "="); ok {
				m[k] = v
			}
		}
		return m
	}

	want := envFor(chrubyScript)
	got := envFor(rpupScript)
	for _, name := range vars {
		if got[name] != want[name] {
			t.Errorf("%s: rpup=%q chruby=%q", name, got[name], want[name])
		}
	}
}

func assertLine(t *testing.T, out, want string) {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == want {
			return
		}
	}
	t.Errorf("expected line %q in output:\n%s", want, out)
}
