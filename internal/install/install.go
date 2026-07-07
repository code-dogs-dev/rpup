// Package install downloads a pre-built ruby tarball and unpacks it into the
// rubies directory, so `rpup install <version>` yields a ruby that Discover can
// find — no compiler, no ruby-install.
package install

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// These pin the single platform/release rpup can install today; broadening to
// other platforms and ruby versions is a later phase.
const (
	releaseTag = "20260520"
	platform   = "arm64_sonoma"
)

// URL is the download location for a pre-built ruby of the given version.
func URL(version string) string {
	return fmt.Sprintf(
		"https://github.com/spinel-coop/rv-ruby/releases/download/%s/ruby-%s.%s.tar.gz",
		releaseTag, version, platform)
}

// Install downloads the pre-built ruby and unpacks it to
// <rubiesDir>/ruby-<version>, returning the install path.
func Install(version, rubiesDir string, force bool, client *http.Client) (string, error) {
	return install(URL(version), version, rubiesDir, force, client)
}

// install is the network-injectable core: extract the tarball at url into
// <rubiesDir>/ruby-<version>. The archive nests the ruby root two levels deep
// (rv-ruby@<v>/<v>/…), so we strip those two leading path components. When force
// is set an existing install is replaced, but only after the new copy has been
// fully extracted, so a failed download never destroys the ruby already there.
func install(url, version, rubiesDir string, force bool, client *http.Client) (string, error) {
	dest := filepath.Join(rubiesDir, "ruby-"+version)
	_, statErr := os.Stat(dest)
	exists := statErr == nil
	if exists && !force {
		return "", fmt.Errorf("ruby %s already installed at %s (use --force to reinstall)", version, dest)
	}
	if err := os.MkdirAll(rubiesDir, 0o750); err != nil {
		return "", err
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: HTTP %d", url, resp.StatusCode)
	}

	tmp, err := os.MkdirTemp(rubiesDir, ".rpup-install-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := extract(resp.Body, tmp, 2); err != nil {
		return "", err
	}
	if exists {
		if err := os.RemoveAll(dest); err != nil {
			return "", err
		}
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// extract untars a gzip stream into dest, dropping the first `strip` leading
// path components of each entry.
func extract(r io.Reader, dest string, strip int) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		rel := stripComponents(hdr.Name, strip)
		if rel == "" {
			continue
		}
		target := filepath.Join(dest, rel)
		if !within(dest, target) {
			return fmt.Errorf("tar entry escapes destination: %s", hdr.Name)
		}

		mode := hdr.FileInfo().Mode().Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(tr, target, mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
}

func writeFile(r io.Reader, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o600) //#nosec G304 -- target is confined to dest by within()
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil { //#nosec G110 -- trusted release tarball, not attacker-controlled
		_ = f.Close()
		return err
	}
	return f.Close()
}

func stripComponents(name string, strip int) string {
	parts := strings.Split(strings.Trim(name, "/"), "/")
	if len(parts) <= strip {
		return ""
	}
	return filepath.Join(parts[strip:]...)
}

// within reports whether target is inside dir, guarding against path-traversal
// entries in the archive.
func within(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
