// Command rpup is a fork-free, drop-in replacement for chruby: it prints shell
// to eval for switching Ruby versions, and generates the shell hook that wires
// per-directory auto-switching.
package main

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/sebjacobs/rpup/internal/install"
	"github.com/sebjacobs/rpup/internal/ruby"
	"github.com/sebjacobs/rpup/internal/shell"
)

var version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		return cmdList(stdout)
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "-V", "--version":
		fmt.Fprintf(stdout, "rpup %s\n", version)
		return 0
	case "list", "ls":
		return cmdList(stdout)
	case "use":
		return cmdUse(args[1:], stdout, stderr)
	case "install":
		return cmdInstall(args[1:], stdout, stderr)
	case "reset":
		fmt.Fprint(stdout, ruby.Render(ruby.EnvFromOS().Reset()))
		return 0
	case "hook":
		return cmdHook(args[1:], stdout, stderr)
	case "doctor":
		return cmdDoctor(stdout, stderr)
	default:
		fmt.Fprintf(stderr, "rpup: unknown command: %s\n%s", args[0], usage)
		return 1
	}
}

// rubiesDir is the one place rpup resolves where rubies live, so install writes
// to and list/use read from exactly the same directory.
func rubiesDir() string {
	return ruby.RubiesDir(os.Getenv(ruby.EnvRubies), os.Getenv("HOME"))
}

func searchDirs() []string {
	return []string{rubiesDir()}
}

func cmdList(stdout *os.File) int {
	active := os.Getenv("RUBY_ROOT")
	for _, r := range ruby.Discover(searchDirs()) {
		mark := " "
		if r.Root == active {
			mark = "*"
		}
		fmt.Fprintf(stdout, " %s %s\n", mark, r.Name())
	}
	return 0
}

func cmdUse(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: rpup use <version|system> [rubyopt...]\n")
		return 1
	}
	query, opts := args[0], strings.Join(args[1:], " ")
	env := ruby.EnvFromOS()
	if query == "system" {
		fmt.Fprint(stdout, ruby.Render(env.Reset()))
		return 0
	}
	match, ok := ruby.Match(ruby.Discover(searchDirs()), query)
	if !ok {
		fmt.Fprintf(stderr, "rpup: unknown Ruby: %s\n", query)
		return 1
	}
	fmt.Fprint(stdout, ruby.Render(env.Use(match, opts)))
	return 0
}

func cmdInstall(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: rpup install <version>\n")
		return 1
	}
	version := args[0]
	dir := rubiesDir()
	fmt.Fprintf(stderr, "rpup: installing ruby %s into %s...\n", version, dir)
	path, err := install.Install(version, dir, http.DefaultClient)
	if err != nil {
		fmt.Fprintf(stderr, "rpup: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "installed ruby %s at %s\n", version, path)
	return 0
}

func cmdHook(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: rpup hook <zsh|bash>\n")
		return 1
	}
	code, err := shell.Hook(args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprint(stdout, code)
	return 0
}

func cmdDoctor(stdout, stderr *os.File) int {
	rubies := ruby.Discover(searchDirs())
	if len(rubies) == 0 {
		fmt.Fprintf(stderr, "rpup: no rubies found under %s\n", strings.Join(searchDirs(), ", "))
		return 1
	}
	active := os.Getenv("RUBY_ROOT")
	if active == "" {
		fmt.Fprintln(stdout, "rpup: no ruby active (system ruby in use)")
		return 0
	}
	want := active + "/bin"
	if !slices.Contains(strings.Split(os.Getenv("PATH"), ":"), want) {
		fmt.Fprintf(stderr, "rpup: RUBY_ROOT is %s but %s is not on PATH — switch did not take effect\n", active, want)
		return 1
	}
	fmt.Fprintf(stdout, "rpup: ok — %s active and on PATH\n", active)
	return 0
}

const usage = `usage: rpup <command> [args]

commands:
  list                 list installed rubies (* = active)
  use <ver> [opts...]  print shell to activate a ruby (eval me)
  use system           print shell to reset to system ruby
  install <version>    download a pre-built ruby into $RPUP_RUBIES (~/.rubies)
  reset                alias for 'use system'
  hook <zsh|bash>      print shell integration (eval me in your rc)
  doctor               check the active ruby actually landed on PATH
  -V, --version        print version
  -h, --help           this help
`
