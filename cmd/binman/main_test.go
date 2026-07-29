package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// pkgTarGz builds a gzipped tar laid out as ./<pkg>/bin/<pkg>, matching the CI
// archive format (top-level dir = package name, stripped on extract).
func pkgTarGz(t *testing.T, pkg string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := "#!/bin/sh\necho " + pkg + "\n"
	if err := tw.WriteHeader(&tar.Header{Name: "./" + pkg + "/bin/" + pkg, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// startRegistry stands up an in-process OCI registry (go-containerregistry),
// pushes one single-layer image per package as <name>-<arch>, points
// ociRegistry at it, and returns nothing (cleanup is registered on t).
func startRegistry(t *testing.T, arch string, packages ...string) {
	t.Helper()
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	repo := u.Host + "/binman"

	for _, name := range packages {
		layer, err := tarball.LayerFromReader(bytes.NewReader(pkgTarGz(t, name)))
		if err != nil {
			t.Fatal(err)
		}
		img, err := mutate.AppendLayers(empty.Image, layer)
		if err != nil {
			t.Fatal(err)
		}
		if err := crane.Push(img, repo+":"+name+"-"+arch); err != nil {
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

func TestExtractLinkRelocate(t *testing.T) {
	root := t.TempDir()
	prefix := filepath.Join(root, "opt", "binman")
	pkg := "ripgrep"

	tgz := filepath.Join(root, "cache", pkg+".tar.gz")
	if err := os.MkdirAll(filepath.Dir(tgz), 0o755); err != nil {
		t.Fatal(err)
	}
	// Reuse the CI-shaped archive, but add a second file to exercise nesting.
	writeArchive(t, tgz, pkg, map[string]string{
		"bin/rg":         "#!/bin/sh\necho rg\n",
		"share/man/rg.1": "manpage\n",
	})

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

// writeArchive builds a gzipped tar of ./<pkg>/<files...> for extraction tests.
func writeArchive(t *testing.T, dst, pkg string, files map[string]string) {
	t.Helper()
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "./" + pkg + "/" + name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
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
	if err := cmdSync(prefix, arch, file, true, false, false); err != nil {
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
	if err := cmdSync(filepath.Join(root, "ignored"), arch, file, false, false, false); err != nil {
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
	if err := cmdSync(flagPrefix, arch, file, true, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(flagPrefix, "bin", "ripgrep")); err != nil {
		t.Errorf("explicit --prefix should override manifest prefix: %v", err)
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
	if err := cmdSync(prefix, arch, file, true, false, true); err != nil {
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
	if err := cmdSync(prefix, arch, file, true, false, false); err != nil {
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
	if err := cmdSync(prefix, arch, file, true, false, false); err != nil {
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

	got := m.installPlan(m.profilePackages())
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
	if err := cmdSync(prefix, arch, file, true, false, false); err != nil {
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
