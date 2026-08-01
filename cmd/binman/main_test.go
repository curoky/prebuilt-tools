package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type archiveEntry struct {
	name     string
	body     string
	linkname string
	typeflag byte
	mode     int64
}

func archiveTarGz(t *testing.T, pkg string, entries ...archiveEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		if entry.typeflag == 0 {
			entry.typeflag = tar.TypeReg
		}
		if entry.mode == 0 {
			entry.mode = 0o755
		}
		header := &tar.Header{
			Name:     "./" + pkg + "/" + entry.name,
			Mode:     entry.mode,
			Size:     int64(len(entry.body)),
			Typeflag: entry.typeflag,
			Linkname: entry.linkname,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if entry.body != "" {
			if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func pkgTarGz(t *testing.T, pkg string) []byte {
	t.Helper()
	return archiveTarGz(t, pkg, archiveEntry{
		name: "bin/" + pkg,
		body: "#!/bin/sh\necho " + pkg + "\n",
	})
}

// startRegistry stands up an in-process OCI registry (go-containerregistry),
// pushes one single-layer image per package as <name>-<arch>, points
// ociRegistry at it, and returns nothing (cleanup is registered on t).
func startRegistry(t *testing.T, arch string, packages ...string) {
	t.Helper()
	startRegistryWithMiddleware(t, arch, nil, packages...)
}

func startRegistryWithMiddleware(
	t *testing.T,
	arch string,
	middleware func(http.Handler) http.Handler,
	packages ...string,
) {
	t.Helper()
	var handler http.Handler = registry.New()
	if middleware != nil {
		handler = middleware(handler)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	repo := u.Host + "/binman"

	for _, packageName := range packages {
		layer, err := tarball.LayerFromReader(bytes.NewReader(pkgTarGz(t, packageName)))
		if err != nil {
			t.Fatal(err)
		}
		img, err := mutate.AppendLayers(empty.Image, layer)
		if err != nil {
			t.Fatal(err)
		}
		reference, err := name.ParseReference(repo+":"+packageName+"-"+arch, name.Insecure)
		if err != nil {
			t.Fatal(err)
		}
		if err := remote.Write(reference, img); err != nil {
			t.Fatal(err)
		}
	}

	old := ociRegistry
	ociRegistry = repo
	t.Cleanup(func() { ociRegistry = old })
}

func TestStripFirstComponent(t *testing.T) {
	cases := map[string]string{
		"./ripgrep/bin/rg":      "bin/rg",
		"ripgrep/share/man/x.1": "share/man/x.1",
		"./ripgrep":             "",
		"ripgrep":               "",
	}
	for in, want := range cases {
		if got := stripFirstComponent(in); got != want {
			t.Errorf("stripFirstComponent(%q)=%q want %q", in, got, want)
		}
	}
}

func TestBinmanNaming(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/cache")

	if defaultPrefix != "/opt/binman" {
		t.Fatalf("defaultPrefix=%q want /opt/binman", defaultPrefix)
	}
	if metaFile != ".binman-meta" {
		t.Fatalf("metaFile=%q want .binman-meta", metaFile)
	}
	if logFile != "binman.log" {
		t.Fatalf("logFile=%q want binman.log", logFile)
	}
	if defaultManifest != "binman.yaml" {
		t.Fatalf("defaultManifest=%q want binman.yaml", defaultManifest)
	}
	wantCache := filepath.Join("/tmp/cache", "binman", "linux-x86_64", "ripgrep.tar.gz")
	if got := cachePath("linux-x86_64", "ripgrep"); got != wantCache {
		t.Fatalf("cachePath=%q want %q", got, wantCache)
	}
}

func TestDownloadPackagesExtractsWithoutInstallState(t *testing.T) {
	const arch = "linux-x86_64"
	startRegistry(t, arch, "wget", "ripgrep")

	output := t.TempDir()
	if err := downloadPackages([]string{"wget", "ripgrep", "wget"}, arch, output); err != nil {
		t.Fatal(err)
	}

	for _, packageName := range []string{"wget", "ripgrep"} {
		binary := filepath.Join(output, packageName, "bin", packageName)
		if _, err := os.Stat(binary); err != nil {
			t.Fatalf("%s was not extracted: %v", packageName, err)
		}
		if _, err := os.Lstat(filepath.Join(output, packageName, metaFile)); !os.IsNotExist(err) {
			t.Fatalf("%s contains installation metadata", packageName)
		}
	}
}

func TestDownloadPackagesRefusesExistingTarget(t *testing.T) {
	const arch = "linux-x86_64"
	output := t.TempDir()
	target := filepath.Join(output, "wget")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}

	err := downloadPackages([]string{"wget"}, arch, output)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite existing path") {
		t.Fatalf("downloadPackages error=%v", err)
	}
}

func TestExtractLinkRelocate(t *testing.T) {
	root := t.TempDir()
	prefix := filepath.Join(root, "opt", "binman")
	pkg := "ripgrep"

	tgz := filepath.Join(root, "cache", pkg+".tar.gz")
	if err := os.MkdirAll(filepath.Dir(tgz), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tgz, archiveTarGz(t, pkg,
		archiveEntry{name: "bin/rg", body: "#!/bin/sh\necho rg\n"},
		archiveEntry{name: "share/man/rg.1", body: "manpage\n"},
	), 0o644); err != nil {
		t.Fatal(err)
	}

	store := storePath(prefix, pkg)
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(tgz, store); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(prefix, meta{Name: pkg, Arch: "linux-x86_64", Digest: "sha256:abc", Linked: true}); err != nil {
		t.Fatal(err)
	}
	if err := linkPkg(prefix, pkg); err != nil {
		t.Fatal(err)
	}

	binLink := filepath.Join(prefix, "bin", "rg")
	target, err := os.Readlink(binLink)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(target) {
		t.Fatalf("symlink target is absolute: %q", target)
	}
	if _, err := os.Stat(binLink); err != nil {
		t.Fatalf("bin link does not resolve: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(prefix, metaFile)); !os.IsNotExist(err) {
		t.Fatalf(".binman-meta leaked into prefix")
	}

	// Relocate the whole prefix; relative links must still resolve.
	moved := filepath.Join(root, "moved", "binman")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(prefix, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(moved, "bin", "rg")); err != nil {
		t.Fatalf("link broken after moving prefix: %v", err)
	}

	if err := unlinkPkg(moved, pkg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(moved, "bin", "rg")); !os.IsNotExist(err) {
		t.Fatalf("link not removed by unlink")
	}
}

func TestReadWriteMeta(t *testing.T) {
	prefix := t.TempDir()
	if err := os.MkdirAll(storePath(prefix, "fd"), 0o755); err != nil {
		t.Fatal(err)
	}
	in := meta{Name: "fd", Arch: "linux-x86_64", Digest: "sha256:deadbeef", Linked: false}
	if err := writeMeta(prefix, in); err != nil {
		t.Fatal(err)
	}
	out, err := readMeta(prefix, "fd")
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != in.Name || out.Arch != in.Arch || out.Digest != in.Digest || out.Linked != in.Linked {
		t.Errorf("roundtrip mismatch: %+v vs %+v", out, in)
	}
}

func TestInstallMultiAllPresent(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep", "fd")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	if err := installPackages([]string{"ripgrep", "fd"}, installOpts{prefix: prefix, arch: arch, linked: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ripgrep", "fd"} {
		if _, err := os.Stat(filepath.Join(prefix, "bin", name)); err != nil {
			t.Errorf("%s not installed/linked: %v", name, err)
		}
		m, err := readMeta(prefix, name)
		if err != nil || m.Name != name {
			t.Errorf("%s metadata missing: %v", name, err)
		}
		if !strings.HasPrefix(m.Digest, "sha256:") {
			t.Errorf("%s digest not recorded: %q", name, m.Digest)
		}
	}
}

// A missing package in the batch must abort the whole install before anything
// is written to the prefix.
func TestInstallMultiOneMissingAbortsAll(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep") // "nope" intentionally absent
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	err := installPackages([]string{"ripgrep", "nope"}, installOpts{prefix: prefix, arch: arch, linked: true})
	if err == nil {
		t.Fatal("expected error for missing package")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should name the missing package, got: %v", err)
	}
	if _, statErr := os.Stat(storePath(prefix, "ripgrep")); !os.IsNotExist(statErr) {
		t.Errorf("ripgrep should NOT be installed when a sibling is missing")
	}
}

func TestInstallCleansLeftoverStoreWithInvalidMetadata(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store := storePath(prefix, "ripgrep")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, metaFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "stale"), []byte("leftover"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installPackages([]string{"ripgrep"}, installOpts{
		prefix: prefix, arch: arch, linked: true,
	}); err != nil {
		t.Fatalf("install should clean the leftover and succeed: %v", err)
	}
	// The stale leftover is gone and a fresh install is in place.
	if _, err := os.Stat(filepath.Join(store, "stale")); !os.IsNotExist(err) {
		t.Fatalf("leftover file was not cleaned: %v", err)
	}
	if _, err := readMeta(prefix, "ripgrep"); err != nil {
		t.Fatalf("valid metadata not written after reinstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(prefix, "bin", "ripgrep")); err != nil {
		t.Fatalf("ripgrep not installed/linked after cleanup: %v", err)
	}
}

func TestInstallDownloadFailureReturns(t *testing.T) {
	arch := "linux-x86_64"
	var truncate atomic.Bool
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if !truncate.Load() || request.Method != http.MethodGet ||
				!strings.Contains(request.URL.Path, "/blobs/") {
				next.ServeHTTP(writer, request)
				return
			}
			recorder := httptest.NewRecorder()
			next.ServeHTTP(recorder, request)
			for key, values := range recorder.Header() {
				writer.Header()[key] = values
			}
			writer.WriteHeader(recorder.Code)
			body := recorder.Body.Bytes()
			if len(body) > 1 {
				body = body[:len(body)/2]
			}
			_, _ = writer.Write(body)
		})
	}
	startRegistryWithMiddleware(t, arch, middleware, "ripgrep")
	truncate.Store(true)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	prefix := t.TempDir()

	done := make(chan error, 1)
	go func() {
		done <- installPackages([]string{"ripgrep"}, installOpts{
			prefix: prefix, arch: arch, linked: true,
		})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected truncated download to fail")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("install hung after a truncated download")
	}
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binman.yaml")
	body := "arch: linux-x86_64\npackages:\n  link:\n    - ripgrep\n    - fd\n  unlink:\n    - python311\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := loadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Arch != "linux-x86_64" {
		t.Errorf("arch=%q want linux-x86_64", m.Arch)
	}
	if len(m.Packages.Link) != 2 || m.Packages.Link[0] != "ripgrep" || m.Packages.Link[1] != "fd" {
		t.Errorf("packages.link=%v want [ripgrep fd]", m.Packages.Link)
	}
	if len(m.Packages.Unlink) != 1 || m.Packages.Unlink[0] != "python311" {
		t.Errorf("packages.unlink=%v want [python311]", m.Packages.Unlink)
	}

	// An empty packages list is an error.
	empty := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(empty, []byte("arch: linux-x86_64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(empty); err == nil {
		t.Error("expected error for manifest with no packages")
	}

	unknown := filepath.Join(dir, "unknown.yaml")
	if err := os.WriteFile(unknown, []byte("packages:\n  links:\n    - ripgrep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(unknown); err == nil {
		t.Error("expected error for unknown manifest field")
	}

	unsafe := filepath.Join(dir, "unsafe.yaml")
	if err := os.WriteFile(unsafe, []byte("profiles:\n  ../../outside:\n    - ripgrep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(unsafe); err == nil {
		t.Error("expected error for unsafe profile name")
	}

	multiple := filepath.Join(dir, "multiple.yaml")
	if err := os.WriteFile(multiple, []byte("packages:\n  link:\n    - ripgrep\n---\npackages:\n  link:\n    - fd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(multiple); err == nil {
		t.Error("expected error for multiple YAML documents")
	}
}

func TestSyncInstalls(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep", "fd")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	file := filepath.Join(t.TempDir(), "binman.yaml")
	if err := os.WriteFile(file, []byte("packages:\n  link:\n    - ripgrep\n    - fd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync(prefix, arch, file, true, false, false, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ripgrep", "fd"} {
		if _, err := os.Stat(filepath.Join(prefix, "bin", name)); err != nil {
			t.Errorf("%s not installed/linked: %v", name, err)
		}
		if _, err := readMeta(prefix, name); err != nil {
			t.Errorf("%s metadata missing: %v", name, err)
		}
	}
}

func TestSyncManifestPrefix(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep")
	root := t.TempDir()
	manifestPrefix := filepath.Join(root, "opt", "binman")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	file := filepath.Join(t.TempDir(), "binman.yaml")
	body := "prefix: " + manifestPrefix + "\npackages:\n  link:\n    - ripgrep\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// prefixSet=false: the flag prefix is a throwaway; the manifest's prefix wins.
	if err := cmdSync(filepath.Join(root, "ignored"), arch, file, false, false, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(manifestPrefix, "bin", "ripgrep")); err != nil {
		t.Errorf("ripgrep not installed under manifest prefix: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "ignored", "bin", "ripgrep")); !os.IsNotExist(err) {
		t.Errorf("nothing should be installed under the flag prefix when manifest prefix is set")
	}

	// prefixSet=true: an explicit --prefix overrides the manifest.
	flagPrefix := filepath.Join(root, "flag", "binman")
	if err := cmdSync(flagPrefix, arch, file, true, false, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(flagPrefix, "bin", "ripgrep")); err != nil {
		t.Errorf("explicit --prefix should override manifest prefix: %v", err)
	}
}

func TestSyncManifestArchPrecedence(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	file := filepath.Join(t.TempDir(), "binman.yaml")
	if err := os.WriteFile(file, []byte("arch: darwin-arm64\npackages:\n  link:\n    - ripgrep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cmdSync(prefix, arch, file, true, true, false, false); err != nil {
		t.Fatal(err)
	}
	got, err := readMeta(prefix, "ripgrep")
	if err != nil {
		t.Fatal(err)
	}
	if got.Arch != arch {
		t.Fatalf("sync arch=%q want explicit flag arch %q", got.Arch, arch)
	}
}

func TestSyncPrune(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep", "fd", "bat")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// Pre-install bat; it is intentionally absent from the manifest below.
	if err := installPackages([]string{"bat"}, installOpts{prefix: prefix, arch: arch, linked: true}); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(t.TempDir(), "binman.yaml")
	if err := os.WriteFile(file, []byte("packages:\n  link:\n    - ripgrep\n    - fd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync(prefix, arch, file, true, false, false, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ripgrep", "fd"} {
		if _, err := os.Stat(filepath.Join(prefix, "bin", name)); err != nil {
			t.Errorf("%s not installed by sync: %v", name, err)
		}
	}
	if _, err := os.Stat(storePath(prefix, "bat")); !os.IsNotExist(err) {
		t.Errorf("bat should have been pruned")
	}
	if _, err := os.Lstat(filepath.Join(prefix, "bin", "bat")); !os.IsNotExist(err) {
		t.Errorf("bat bin link should have been removed by prune")
	}
}

func TestSyncUnlinked(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep", "python311")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	file := filepath.Join(t.TempDir(), "binman.yaml")
	body := "packages:\n  link:\n    - ripgrep\n  unlink:\n    - python311\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync(prefix, arch, file, true, false, false, false); err != nil {
		t.Fatal(err)
	}
	// Regular package is linked into the prefix root.
	if _, err := os.Stat(filepath.Join(prefix, "bin", "ripgrep")); err != nil {
		t.Errorf("ripgrep not linked into prefix root: %v", err)
	}
	// Unlinked package lands in the store but is NOT linked into the root.
	if _, err := os.Stat(storePath(prefix, "python311")); err != nil {
		t.Errorf("python311 not installed into store: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(prefix, "bin", "python311")); !os.IsNotExist(err) {
		t.Errorf("python311 should NOT be linked into prefix root")
	}
}

func TestSyncProfiles(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep", "gopls", "delve")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	file := filepath.Join(t.TempDir(), "binman.yaml")
	body := "packages:\n  link:\n    - ripgrep\nprofiles:\n  go:\n    - gopls\n    - delve\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync(prefix, arch, file, true, false, false, false); err != nil {
		t.Fatal(err)
	}

	// The regular package is linked into the prefix root.
	if _, err := os.Stat(filepath.Join(prefix, "bin", "ripgrep")); err != nil {
		t.Errorf("ripgrep not linked into prefix root: %v", err)
	}
	// Profile packages are installed into the store but NOT linked into root.
	for _, name := range []string{"gopls", "delve"} {
		if _, err := os.Stat(storePath(prefix, name)); err != nil {
			t.Errorf("%s not installed into store: %v", name, err)
		}
		if _, err := os.Lstat(filepath.Join(prefix, "bin", name)); !os.IsNotExist(err) {
			t.Errorf("%s should NOT be linked into prefix root", name)
		}
		// ... they are exposed under the profile directory instead.
		link := filepath.Join(prefix, "profile", "go", "bin", name)
		if _, err := os.Stat(link); err != nil {
			t.Errorf("%s not linked into profile go: %v", name, err)
		}
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.IsAbs(target) {
			t.Errorf("profile link target is absolute: %q", target)
		}
	}
}

func TestManifestInstallPlan(t *testing.T) {
	m := manifest{
		Packages: packageSet{
			Link:   []string{"ripgrep", "fd"},
			Unlink: []string{"python311", "ripgrep"},
		},
		Profiles: map[string][]string{
			"go": {"gopls", "fd"},
			"js": {"nodejs", "python311"},
		},
	}

	got := m.installPlan()
	want := []installTarget{
		{name: "ripgrep", linked: true},
		{name: "fd", linked: true},
		{name: "python311", linked: false},
		{name: "gopls", linked: false},
		{name: "nodejs", linked: false},
	}
	if len(got) != len(want) {
		t.Fatalf("plan length=%d want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("plan[%d]=%#v want %#v", i, got[i], want[i])
		}
	}
}

func TestSyncSharedBatchKeepsLinkedPackagesLinked(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	file := filepath.Join(t.TempDir(), "binman.yaml")
	body := "packages:\n  link:\n    - ripgrep\n  unlink:\n    - ripgrep\nprofiles:\n  tools:\n    - ripgrep\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync(prefix, arch, file, true, false, false, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(storePath(prefix, "ripgrep")); err != nil {
		t.Fatalf("ripgrep not installed into store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(prefix, "bin", "ripgrep")); err != nil {
		t.Fatalf("ripgrep not linked into prefix root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(prefix, "profile", "tools", "bin", "ripgrep")); err != nil {
		t.Fatalf("ripgrep not linked into profile root: %v", err)
	}
	m, err := readMeta(prefix, "ripgrep")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Linked {
		t.Fatalf("ripgrep metadata should record linked=true, got %#v", m)
	}
}

func TestRemoveRejectsPackagePathTraversal(t *testing.T) {
	root := t.TempDir()
	prefix := filepath.Join(root, "prefix")
	victim := filepath.Join(root, "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "keep"), []byte("important"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cmdRemove(prefix, "../../victim"); err == nil {
		t.Fatal("expected invalid package name error")
	}
	if _, err := os.Stat(filepath.Join(victim, "keep")); err != nil {
		t.Fatalf("remove escaped the store and deleted victim: %v", err)
	}
}

func TestRemovePreservesStoreWhenMetadataIsInvalid(t *testing.T) {
	prefix := t.TempDir()
	name := "ripgrep"
	store := storePath(prefix, name)
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, metaFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "keep"), []byte("important"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cmdRemove(prefix, name); err == nil {
		t.Fatal("expected invalid metadata error")
	}
	if _, err := os.Stat(filepath.Join(store, "keep")); err != nil {
		t.Fatalf("remove deleted store with invalid metadata: %v", err)
	}
}

func TestExtractRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "escape.tar.gz")
	if err := os.WriteFile(archive, archiveTarGz(t, "pkg",
		archiveEntry{
			name: "link", typeflag: tar.TypeSymlink, linkname: "../outside", mode: 0o777,
		},
		archiveEntry{
			name: "link/written-outside", body: "escaped", mode: 0o644,
		},
	), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := extractTarGz(archive, filepath.Join(root, "store")); err == nil {
		t.Fatal("expected escaping symlink to be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "outside", "written-outside")); !os.IsNotExist(err) {
		t.Fatalf("archive wrote outside extraction root: %v", err)
	}
}

func TestExtractPreservesInternalRelativeSymlink(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "symlink.tar.gz")
	if err := os.WriteFile(archive, archiveTarGz(t, "pkg",
		archiveEntry{name: "lib/target", body: "target", mode: 0o644},
		archiveEntry{
			name: "bin/tool", typeflag: tar.TypeSymlink, linkname: "../lib/target", mode: 0o777,
		},
	), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(root, "store")
	if err := extractTarGz(archive, dst); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(dst, "bin", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "../lib/target" {
		t.Fatalf("symlink target=%q want ../lib/target", target)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "bin", "tool")); err != nil || string(got) != "target" {
		t.Fatalf("internal symlink does not resolve: content=%q err=%v", got, err)
	}
}

func TestLinkConflictDoesNotOverwriteOwner(t *testing.T) {
	prefix := t.TempDir()
	for _, name := range []string{"first", "second"} {
		path := filepath.Join(storePath(prefix, name), "bin", "tool")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := linkPkg(prefix, "first"); err != nil {
		t.Fatal(err)
	}
	if err := linkPkg(prefix, "second"); err == nil {
		t.Fatal("expected link ownership conflict")
	}

	target, err := os.Readlink(filepath.Join(prefix, "bin", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(target, "first") {
		t.Fatalf("conflicting package replaced existing owner: %q", target)
	}
}

func TestLinkMergesPackageDirectorySymlinks(t *testing.T) {
	prefix := t.TempDir()
	for _, name := range []string{"first", "second"} {
		path := filepath.Join(storePath(prefix, name), "bin", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("bin", filepath.Join(storePath(prefix, name), "sbin")); err != nil {
			t.Fatal(err)
		}
	}

	if err := linkPkg(prefix, "first"); err != nil {
		t.Fatal(err)
	}
	if err := linkPkg(prefix, "second"); err != nil {
		t.Fatal(err)
	}

	sbinInfo, err := os.Lstat(filepath.Join(prefix, "sbin"))
	if err != nil {
		t.Fatal(err)
	}
	if !sbinInfo.IsDir() || sbinInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("sbin mode=%v want mergeable directory", sbinInfo.Mode())
	}
	for _, dir := range []string{"bin", "sbin"} {
		for _, name := range []string{"first", "second"} {
			if got, err := os.ReadFile(filepath.Join(prefix, dir, name)); err != nil || string(got) != name {
				t.Fatalf("%s/%s content=%q err=%v", dir, name, got, err)
			}
		}
	}

	if err := unlinkPkg(prefix, "first"); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"bin", "sbin"} {
		if _, err := os.Lstat(filepath.Join(prefix, dir, "first")); !os.IsNotExist(err) {
			t.Fatalf("%s/first remains after unlink: %v", dir, err)
		}
		if got, err := os.ReadFile(filepath.Join(prefix, dir, "second")); err != nil || string(got) != "second" {
			t.Fatalf("%s/second content=%q err=%v", dir, got, err)
		}
	}
}

func TestLinkMigratesManagedDirectorySymlink(t *testing.T) {
	prefix := t.TempDir()
	for _, item := range []struct {
		name string
		tool string
	}{
		{name: "nettools", tool: "route"},
		{name: "iproute2", tool: "ip"},
	} {
		path := filepath.Join(storePath(prefix, item.name), "bin", item.tool)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(item.name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("bin", filepath.Join(storePath(prefix, item.name), "sbin")); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	route := filepath.Join(storePath(prefix, "nettools"), "bin", "route")
	binTarget, err := filepath.Rel(filepath.Join(prefix, "bin"), route)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binTarget, filepath.Join(prefix, "bin", "route")); err != nil {
		t.Fatal(err)
	}
	sbinTarget, err := filepath.Rel(prefix, filepath.Join(storePath(prefix, "nettools"), "sbin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sbinTarget, filepath.Join(prefix, "sbin")); err != nil {
		t.Fatal(err)
	}

	if err := linkPkg(prefix, "iproute2"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(prefix, "sbin"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("sbin mode=%v want migrated directory", info.Mode())
	}
	for _, item := range []struct {
		tool string
		want string
	}{
		{tool: "route", want: "nettools"},
		{tool: "ip", want: "iproute2"},
	} {
		if got, err := os.ReadFile(filepath.Join(prefix, "sbin", item.tool)); err != nil || string(got) != item.want {
			t.Fatalf("sbin/%s content=%q err=%v", item.tool, got, err)
		}
	}
}

func TestUnlinkRemovesOwnedLegacyDirectorySymlink(t *testing.T) {
	prefix := t.TempDir()
	name := "nettools"
	path := filepath.Join(storePath(prefix, name), "bin", "route")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(name), 0o755); err != nil {
		t.Fatal(err)
	}
	storeSbin := filepath.Join(storePath(prefix, name), "sbin")
	if err := os.Symlink("bin", storeSbin); err != nil {
		t.Fatal(err)
	}
	target, err := filepath.Rel(prefix, storeSbin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(prefix, "sbin")); err != nil {
		t.Fatal(err)
	}

	if err := unlinkPkg(prefix, name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(prefix, "sbin")); !os.IsNotExist(err) {
		t.Fatalf("legacy directory symlink remains after unlink: %v", err)
	}
}

func TestLinkRejectsSymlinkParentEscape(t *testing.T) {
	prefix := t.TempDir()
	outside := t.TempDir()
	name := "ripgrep"
	path := filepath.Join(storePath(prefix, name), "bin", "rg")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(prefix, "bin")); err != nil {
		t.Fatal(err)
	}

	if err := linkPkg(prefix, name); err == nil {
		t.Fatal("expected symlink parent to be rejected")
	}
	if _, err := os.Lstat(filepath.Join(outside, "rg")); !os.IsNotExist(err) {
		t.Fatalf("link escaped prefix through symlink parent: %v", err)
	}
}

func TestUnlinkDoesNotDeleteAnotherPackageLink(t *testing.T) {
	prefix := t.TempDir()
	for _, name := range []string{"first", "second"} {
		path := filepath.Join(storePath(prefix, name), "bin", "tool")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dest := filepath.Join(prefix, "bin", "tool")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	target, err := filepath.Rel(filepath.Dir(dest), filepath.Join(storePath(prefix, "second"), "bin", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dest); err != nil {
		t.Fatal(err)
	}
	if err := unlinkPkg(prefix, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("unlink removed another package's link: %v", err)
	}
}

func TestUnlinkRejectsSymlinkParentEscape(t *testing.T) {
	prefix := t.TempDir()
	outside := t.TempDir()
	name := "ripgrep"
	path := filepath.Join(storePath(prefix, name), "bin", "rg")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package"), 0o755); err != nil {
		t.Fatal(err)
	}
	target, err := filepath.Rel(outside, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(outside, "rg")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(prefix, "bin")); err != nil {
		t.Fatal(err)
	}

	if err := unlinkPkg(prefix, name); err == nil {
		t.Fatal("expected symlink parent to be rejected")
	}
	if _, err := os.Lstat(filepath.Join(outside, "rg")); err != nil {
		t.Fatalf("unlink removed path outside prefix: %v", err)
	}
}

func TestUnlinkPreservesUserReplacement(t *testing.T) {
	prefix := t.TempDir()
	name := "ripgrep"
	path := filepath.Join(storePath(prefix, name), "bin", "rg")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package"), 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(prefix, "bin", "rg")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("user"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := unlinkPkg(prefix, name); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "user" {
		t.Fatalf("user replacement changed: %q", got)
	}
}

func TestSyncReconcilesProfiles(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "gopls", "delve")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	file := filepath.Join(t.TempDir(), "binman.yaml")

	if err := os.WriteFile(file, []byte("profiles:\n  go:\n    - gopls\n    - delve\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync(prefix, arch, file, true, false, false, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("profiles:\n  go:\n    - gopls\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync(prefix, arch, file, true, false, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(prefix, "profile", "go", "bin", "delve")); !os.IsNotExist(err) {
		t.Fatalf("stale profile link was not removed: %v", err)
	}
}

func TestRemoveCleansProfileLinks(t *testing.T) {
	prefix := t.TempDir()
	name := "gopls"
	path := filepath.Join(storePath(prefix, name), "bin", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(prefix, meta{Name: name, Arch: "linux-x86_64", Digest: "sha256:test"}); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(prefix, "profile", "go")
	if err := linkPkgInto(prefix, name, profile); err != nil {
		t.Fatal(err)
	}

	if err := cmdRemove(prefix, name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(profile, "bin", name)); !os.IsNotExist(err) {
		t.Fatalf("profile link remains after remove: %v", err)
	}
}

func TestOutdatedReturnsRegistryErrors(t *testing.T) {
	prefix := t.TempDir()
	if err := os.MkdirAll(storePath(prefix, "ripgrep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(prefix, meta{
		Name: "ripgrep", Arch: "linux-x86_64", Digest: "sha256:old",
	}); err != nil {
		t.Fatal(err)
	}
	startRegistry(t, "linux-x86_64")

	if err := cmdOutdated(prefix); err == nil {
		t.Fatal("expected registry error")
	}
}

func TestOutdatedResolvesPackagesConcurrently(t *testing.T) {
	arch := "linux-x86_64"
	var active int32
	var peak int32
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/manifests/") {
				current := atomic.AddInt32(&active, 1)
				for {
					previous := atomic.LoadInt32(&peak)
					if current <= previous || atomic.CompareAndSwapInt32(&peak, previous, current) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				defer atomic.AddInt32(&active, -1)
			}
			next.ServeHTTP(writer, request)
		})
	}
	packages := []string{"ripgrep", "fd", "bat", "eza"}
	startRegistryWithMiddleware(t, arch, middleware, packages...)
	prefix := t.TempDir()
	for _, packageName := range packages {
		if err := os.MkdirAll(storePath(prefix, packageName), 0o755); err != nil {
			t.Fatal(err)
		}
		digest, err := remoteDigest(packageName, arch)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeMeta(prefix, meta{
			Name: packageName, Arch: arch, Digest: digest,
		}); err != nil {
			t.Fatal(err)
		}
	}
	atomic.StoreInt32(&peak, 0)

	if err := cmdOutdated(prefix); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&peak); got < 2 {
		t.Fatalf("remote checks did not overlap: peak concurrency=%d", got)
	}
}

func TestUpgradeResolvesAllPackagesBeforeWriting(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "first")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for _, name := range []string{"first", "missing"} {
		if err := os.MkdirAll(storePath(prefix, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeMeta(prefix, meta{
			Name: name, Arch: arch, Digest: "sha256:old-" + name,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := cmdUpgrade(prefix, arch, []string{"first", "missing"}); err == nil {
		t.Fatal("expected missing package error")
	}
	got, err := readMeta(prefix, "first")
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != "sha256:old-first" {
		t.Fatalf("first package changed before all upgrades resolved: %q", got.Digest)
	}
}

func TestUpgradeArchOverride(t *testing.T) {
	arch := "linux-x86_64"
	startRegistry(t, arch, "ripgrep")
	prefix := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(storePath(prefix, "ripgrep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(prefix, meta{
		Name: "ripgrep", Arch: "darwin-arm64", Digest: "sha256:old",
	}); err != nil {
		t.Fatal(err)
	}

	if err := cmdUpgrade(prefix, arch, []string{"ripgrep"}); err != nil {
		t.Fatal(err)
	}
	got, err := readMeta(prefix, "ripgrep")
	if err != nil {
		t.Fatal(err)
	}
	if got.Arch != arch {
		t.Fatalf("upgrade arch=%q want %q", got.Arch, arch)
	}
}
