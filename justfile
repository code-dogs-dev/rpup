# rpup — local dev tasks.
# Run `just` to list recipes, `just <name>` to invoke one.

# Semantic version, from the nearest git tag (e.g. v0.1.0, or v0.1.0-3-gabc123
# past a tag, +-dirty with uncommitted changes). Falls back to the short commit
# when no tag exists yet, and to "dev" outside a git checkout.
version := `git describe --tags --always --dirty 2>/dev/null || echo dev`

# Where `just install` puts the binary. Override with RPUP_INSTALL_PATH.
install_path := env_var_or_default("RPUP_INSTALL_PATH", home_directory() / ".local/bin/rpup")

default:
    @just --list

# Build the version-stamped binary into bin/.
build:
    go build -ldflags "-X main.version={{version}}" -o bin/rpup .

# Build and install the binary onto your PATH.
install: build
    @mkdir -p "$(dirname "{{install_path}}")"
    cp bin/rpup "{{install_path}}"
    @echo "installed rpup {{version}} -> {{install_path}}"

# Run all tests (unit + the zsh/bash drop-in smoke suite).
test:
    go test ./...

# Run the linter (same config as CI).
lint:
    golangci-lint run

# Regenerate assets/logo-wordmark.png from the mark and the "rpup" wordmark.
wordmark:
    uv run scripts/compose_wordmark.py

# Run every check CI runs — build, test, lint. Use before pushing.
check: build test lint

# Remove build artefacts.
clean:
    rm -rf bin/ dist/
