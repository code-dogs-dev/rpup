# rpup — spec

A fork-free, well-tested drop-in replacement for [chruby](https://github.com/postmodern/chruby)
(`chruby.sh` + `auto.sh`). A small Go binary emits shell to `eval`; the generated
hook keeps the per-command path fork-free and never spawns `ruby`.

## Why

chruby's switch spawns `ruby` (~40ms) purely to read `RUBY_ENGINE`,
`RUBY_VERSION`, and `Gem.default_dir`. All three are derivable without ruby:

- **version / engine** — from the `ruby-X.Y.Z` (or `jruby-…`, `truffleruby-…`) dir name
- **`GEM_ROOT`** — from the `$root/lib/ruby/gems/*` glob (already the proven inline
  workaround in `~/dotfiles/zsh/env.zsh`)

This lets one implementation serve both interactive and non-interactive shells,
replacing the two divergent hand-inlined copies in the dotfiles, and fixes
chruby's silent empty-`RUBIES`-glob no-op (surfaced by `rpup doctor`).

## Ruby search paths

Same as chruby: `$PREFIX/opt/rubies/*` and `$HOME/.rubies/*` (dirs only). A ruby
is any such dir with an executable `bin/ruby`.

## Env contract (identical to chruby, must match byte-for-byte)

On `use <root>`:

```
RUBY_ROOT   = <root>
RUBYOPT     = <passed opts>                 # only if non-empty
RUBY_ENGINE = ruby|jruby|truffleruby|…      # from dir name
RUBY_VERSION= X.Y.Z                          # from dir name
GEM_ROOT    = <root>/lib/ruby/gems/<glob>    # first match; may be absent
GEM_HOME    = $HOME/.gem/$RUBY_ENGINE/$RUBY_VERSION   # only if UID != 0
GEM_PATH    = $GEM_HOME:$GEM_ROOT            # GEM_ROOT segment only if present
PATH        = $GEM_HOME/bin:$GEM_ROOT/bin:$RUBY_ROOT/bin:$PATH
```

`reset` / `use system`: remove exactly `$RUBY_ROOT/bin`, `$GEM_HOME/bin`,
`$GEM_ROOT/bin` from PATH; strip `$GEM_HOME`,`$GEM_ROOT` from GEM_PATH; unset
`RUBY_ROOT RUBY_ENGINE RUBY_VERSION RUBYOPT GEM_ROOT GEM_HOME` (and `GEM_PATH`
if empty). Root (UID 0) never touches GEM_*. Mirrors `chruby_reset`.

## CLI

| command | behaviour |
|---|---|
| `rpup list` | installed rubies, `*` marks active (matches `chruby` no-arg) |
| `rpup use <ver\|fuzzy\|system> [opts…]` | print `export`/`unset` lines to stdout for `eval`. Fuzzy match = chruby's: exact dir-name basename wins, else last substring match |
| `rpup reset` | print the reset lines |
| `rpup hook zsh\|bash` | print the shell function + preexec/chpwd auto wiring |
| `rpup doctor` | verify a ruby actually landed on PATH; loud on the empty-glob case |
| `rpup --version` / `-V` | version |
| `rpup --help` / `-h` | usage |

`use` unknown version → exit 1, message on stderr, nothing on stdout.

## Shell hook (`rpup hook zsh`)

Emits:

- `rpup()` — `eval "$(command rpup use …)"` for switch/reset; passes through
  list/doctor.
- `_rpup_auto()` — **pure shell**, no fork: walk `$PWD` upward for `.ruby-version`,
  compare to `$RPUP_CURRENT_VERSION`; only on change call `rpup use`. Mirrors
  `chruby_auto` so the per-command cost stays a couple of string ops.
- zsh: append to `preexec_functions`; bash: `PROMPT_COMMAND`/`DEBUG` trap
  (match chruby's choice).

## Testing

- **Go unit tests** — version/engine parse, gem-root glob, PATH add/remove,
  fuzzy matching, use/reset output, root vs non-root.
- **Differential smoke tests** — boot real `zsh` and `bash`, source the rpup hook
  *and* the vendored `./tmp/chruby.sh`+`auto.sh` against identical fixture ruby
  dirs + `.ruby-version` files; assert the resulting `RUBY_*`/`GEM_*`/`PATH` are
  identical (modulo the deliberate no-ruby-fork difference). This is the
  drop-in guarantee.

## Out of scope (v1)

Installing/building rubies — `ruby-install` stays the installer. `rpup install`
is a possible later phase.
