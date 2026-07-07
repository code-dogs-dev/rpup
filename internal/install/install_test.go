package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebjacobs/rpup/internal/ruby"
)

func TestURL(t *testing.T) {
	want := "https://github.com/spinel-coop/rv-ruby/releases/download/20260520/ruby-4.0.5.arm64_sonoma.tar.gz"
	if got := URL("4.0.5"); got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

// entry is a file to place in the test tarball.
type entry struct {
	name    string
	body    string
	mode    int64
	symlink string
}

func makeTarball(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: e.mode}
		switch {
		case e.symlink != "":
			hdr.Typeflag, hdr.Linkname = tar.TypeSymlink, e.symlink
		case e.name[len(e.name)-1] == '/':
			hdr.Typeflag = tar.TypeDir
		default:
			hdr.Typeflag, hdr.Size = tar.TypeReg, int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInstall(t *testing.T) {
	// Mirror the real rv-ruby layout: the ruby root is nested two dirs deep.
	tarball := makeTarball(t, []entry{
		{name: "rv-ruby@4.0.5/4.0.5/", mode: 0o755},
		{name: "rv-ruby@4.0.5/4.0.5/bin/", mode: 0o755},
		{name: "rv-ruby@4.0.5/4.0.5/bin/ruby", body: "#!/bin/sh\n", mode: 0o755},
		{name: "rv-ruby@4.0.5/4.0.5/lib/ruby/gems/4.0.0/", mode: 0o755},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	rubies := t.TempDir()
	path, err := install(srv.URL, "4.0.5", rubies, srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(rubies, "ruby-4.0.5"); path != want {
		t.Errorf("install path = %q, want %q", path, want)
	}

	binRuby := filepath.Join(path, "bin", "ruby")
	info, err := os.Stat(binRuby)
	if err != nil {
		t.Fatalf("bin/ruby not extracted: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("bin/ruby not executable: %v", info.Mode())
	}

	// The installed ruby must be discoverable exactly as a chruby-style install.
	found := ruby.Discover([]string{rubies})
	if len(found) != 1 || found[0].Name() != "ruby-4.0.5" {
		t.Errorf("Discover = %+v, want one ruby-4.0.5", found)
	}
}

func TestInstallRejectsExisting(t *testing.T) {
	rubies := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rubies, "ruby-4.0.5"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := install("http://unused", "4.0.5", rubies, http.DefaultClient); err == nil {
		t.Error("installing over an existing ruby should error")
	}
}
