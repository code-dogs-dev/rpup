<p align="center">
  <img src="assets/logo-wordmark.png" alt="rpup" width="420">
</p>

# rpup

**Switch Ruby without spawning one.** `rpup` is a fork-free, drop-in replacement
for [chruby](https://github.com/postmodern/chruby): it switches your Ruby version
by printing shell to `eval`, and never spawns a `ruby` process to do it — so every
switch and every `cd` stays fast.

```
$ rpup ls                 # installed rubies (* = active)
    ruby-3.4.5
  * ruby-4.0.5
$ eval "$(rpup use 3.4.5)" # switch to a version
$ rpup doctor             # confirm the switch actually landed on PATH
rpup: ok — /Users/you/.rubies/ruby-3.4.5 active and on PATH
```

## Why not just chruby?

chruby spawns `ruby` (~40ms) on every switch purely to read three values —
`RUBY_ENGINE`, `RUBY_VERSION`, and `Gem.default_dir`. All three are derivable
without running Ruby at all:

- **version / engine** — from the `ruby-X.Y.Z` (or `jruby-…`, `truffleruby-…`) directory name
- **`GEM_ROOT`** — from the `$root/lib/ruby/gems/*` glob

`rpup` derives them directly, so one implementation serves both interactive and
non-interactive shells with no fork. It matches chruby's environment output
byte-for-byte (verified by a differential smoke suite that boots real `zsh` and
`bash` against both), and it's loud where chruby is silent: `rpup doctor` catches
the empty-`RUBIES`-glob case where a "switch" quietly does nothing.

Ruby search paths are the same as chruby: `$PREFIX/opt/rubies/*` and
`$HOME/.rubies/*`.

## Install

### One-line install

```
curl -sSfL https://raw.githubusercontent.com/code-dogs-dev/rpup/main/install.sh | sh
```

Downloads the latest release for your OS/arch and installs the `rpup` binary to
`/usr/local/bin` (override with `RPUP_INSTALL_DIR`).

### Homebrew

```
brew install code-dogs-dev/tap/rpup
```

### From source

```
just install     # builds and installs rpup to ~/.local/bin (override with RPUP_INSTALL_PATH)
```

### Enable auto-switching

Add the hook to your shell rc so `rpup` switches automatically on `cd` into a
directory with a `.ruby-version`:

```
eval "$(rpup hook zsh)"     # ~/.zshrc   (bash: rpup hook bash in ~/.bashrc)
```

Open a new shell and you're set. Without the hook you can still switch by hand
with `eval "$(rpup use 3.4.5)"`.

## Commands

```
rpup list                  List installed rubies (* = active). The bare `rpup` does this too.
rpup use <ver> [opts...]   Print shell to activate a ruby (eval it). Fuzzy match, chruby-style.
rpup use system            Print shell to reset to the system ruby
rpup reset                 Alias for `use system`
rpup hook <zsh|bash>       Print the shell integration (eval it in your rc)
rpup doctor                Check the active ruby actually landed on PATH
rpup --version / -V        Print the version
rpup --help / -h           Usage
```

## Development

```
just build     # build the version-stamped binary into bin/
just test      # go test ./...  (unit + zsh/bash drop-in smoke suite)
just lint      # golangci-lint (same config as CI)
just check     # build + test + lint — run before pushing
```

Releases are cut by pushing a `vX.Y.Z` tag: [GoReleaser](https://goreleaser.com)
builds the cross-platform binaries, publishes the GitHub release, and bumps the
Homebrew cask.

## License

MIT
