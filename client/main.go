// Command sb is a tiny package manager for the standalone-binaries
// published at ghcr.io/curoky/standalone-binaries.
//
// Design goals (see CLAUDE.md "Client Install / Upgrade Model"):
//
//   - Single static binary. sb is one statically-linked binary (built with
//     CGO_ENABLED=0), cross-compiled for linux-x86_64 and darwin-arm64. OCI
//     access is delegated to go-containerregistry (crane), so neither curl,
//     tar, oras nor jq is required on the target host.
//   - Relocatable installs. Packages live under <prefix>/store/<name> and are
//     exposed through *relative* symlinks in <prefix>/{bin,lib,share,...}.
//     Because every link is relative, the whole prefix can be moved anywhere
//     with zero repair.
//   - Independent packages. Every package is treated as fully self-contained;
//     sb performs no dependency resolution.
//
// Commands: install | remove | upgrade | info | list | outdated | sync
//
// `install` accepts multiple packages and runs in three phases:
//  1. resolve every package's remote layer digest in parallel (a missing
//     package is an error; if any package is missing, nothing is installed);
//  2. download the needed layers in parallel into the cache;
//  3. extract + link them serially.
package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

const (
	defaultRegistry = "ghcr.io/curoky/standalone-binaries"
	metaFile        = ".sb-meta"
	defaultPrefix   = "/opt/sb"
	logFile         = "sb.log"
	maxParallel     = 16 // cap concurrent registry requests / downloads
)

// logger carries the detailed, structured log. It always writes to
// <prefix>/sb.log; with --verbose it also mirrors to stderr. CLI key-step
// output (the "> ..." lines) keeps going straight to stdout via fmt.
var logger = slog.New(slog.NewTextHandler(io.Discard, nil))

// setupLogger points logger at <prefix>/sb.log, creating the prefix if needed.
// When verbose is set, log records are also written to stderr. The returned
// closer flushes/closes the underlying file.
func setupLogger(prefix string, verbose bool) (io.Closer, error) {
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create prefix %s: %w", prefix, err)
	}
	path := filepath.Join(prefix, logFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cannot open log file %s: %w", path, err)
	}
	var w io.Writer = f
	if verbose {
		w = io.MultiWriter(f, os.Stderr)
	}
	logger = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return f, nil
}

// detectArch returns the publish arch tag for the current platform. Only
// linux-x86_64 and darwin-arm64 are published; anything else must be passed
// explicitly via --arch.
func detectArch() (string, error) {
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return "linux-x86_64", nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "darwin-arm64", nil
	}
	return "", fmt.Errorf("unsupported platform %s/%s; pass --arch linux-x86_64 or darwin-arm64",
		runtime.GOOS, runtime.GOARCH)
}

// ---------------------------------------------------------------------------
// OCI registry access (delegated to go-containerregistry / crane).
// ---------------------------------------------------------------------------

// ociRegistry is the registry reference root. It is a variable so tests can
// point it at a local httptest registry; production always uses ghcr.
var ociRegistry = defaultRegistry

func ref(name, arch string) string { return fmt.Sprintf("%s:%s-%s", ociRegistry, name, arch) }

// isNotFound reports whether err is a registry 404 (i.e. the package/tag does
// not exist for the requested arch).
func isNotFound(err error) bool {
	var terr *transport.Error
	return errors.As(err, &terr) && terr.StatusCode == http.StatusNotFound
}

// remoteLayer returns the single content layer of a package's image. The layer
// digest is what we record in .sb-meta and compare for upgrades; the layer's
// Compressed() stream is the package tarball.
func remoteLayer(name, arch string) (v1.Layer, error) {
	img, err := crane.Pull(ref(name, arch))
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%s: not found for arch %q", name, arch)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("%s: image has no layers", name)
	}
	return layers[len(layers)-1], nil
}

// remoteDigest resolves a package's layer digest without downloading content.
func remoteDigest(name, arch string) (string, error) {
	layer, err := remoteLayer(name, arch)
	if err != nil {
		return "", err
	}
	d, err := layer.Digest()
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return d.String(), nil
}

// downloadLayer streams a package's layer tarball into the cache. If wrap is
// non-nil it is called with the compressed layer size and must return a reader
// that reports read progress (e.g. an mpb bar's ProxyReader) wrapping rc; the
// returned closer, if any, is closed when the copy finishes.
func downloadLayer(name, arch, dst string, wrap func(size int64, rc io.ReadCloser) io.ReadCloser) error {
	layer, err := remoteLayer(name, arch)
	if err != nil {
		return err
	}
	rc, err := layer.Compressed()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	var src io.Reader = rc
	if wrap != nil {
		size, err := layer.Size()
		if err != nil {
			return err
		}
		proxy := wrap(size, rc)
		defer proxy.Close()
		src = proxy
	}
	_, err = io.Copy(f, src)
	return err
}

func cachePath(arch, name string) string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(base, "sb", arch, name+".tar.gz")
}

// ---------------------------------------------------------------------------
// Store / metadata / relative symlinks.
// ---------------------------------------------------------------------------

type meta struct {
	Name        string `json:"name"`
	Arch        string `json:"arch"`
	Digest      string `json:"digest"`
	Linked      bool   `json:"linked"`
	InstalledAt string `json:"installed_at"`
}

func metaPath(prefix, name string) string  { return filepath.Join(prefix, "store", name, metaFile) }
func storePath(prefix, name string) string { return filepath.Join(prefix, "store", name) }

func readMeta(prefix, name string) (meta, error) {
	var m meta
	data, err := os.ReadFile(metaPath(prefix, name))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

func writeMeta(prefix string, m meta) error {
	m.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath(prefix, m.Name), data, 0o644)
}

// extractTarGz extracts a gzipped tar into dst, stripping the leading path
// component (CI archives packages as ./<name>/...).
func extractTarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		rel := stripFirstComponent(hdr.Name)
		if rel == "" {
			continue
		}
		target := filepath.Join(dst, rel)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			err = writeFile(target, tr, os.FileMode(hdr.Mode)&0o777)
		case tar.TypeSymlink:
			if err = os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
				_ = os.Remove(target)
				err = os.Symlink(hdr.Linkname, target)
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// writeFile creates target (with parent dirs) and copies r into it.
func writeFile(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

// stripFirstComponent drops the leading path component (e.g. "./ripgrep/bin/rg"
// -> "bin/rg"); returning "" signals the entry should be skipped (the top dir).
func stripFirstComponent(name string) string {
	name = strings.TrimPrefix(filepath.ToSlash(name), "./")
	parts := strings.SplitN(name, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// walkPkgFiles calls fn for every regular file / symlink under the package's
// store dir, skipping the metadata file. relPath is relative to the store dir.
func walkPkgFiles(store string, fn func(absPath, relPath string) error) error {
	return filepath.Walk(store, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(store, p)
		if err != nil {
			return err
		}
		if rel == metaFile {
			return nil
		}
		return fn(p, rel)
	})
}

// linkPkg creates relative symlinks from store/<name>/* into the prefix root.
func linkPkg(prefix, name string) error {
	return linkPkgInto(prefix, name, prefix)
}

// linkPkgInto creates relative symlinks from store/<name>/* into an arbitrary
// destination root. It is the shared mechanism behind both the default
// prefix-root linking and `sync` profile linking.
func linkPkgInto(prefix, name, root string) error {
	return walkPkgFiles(storePath(prefix, name), func(abs, rel string) error {
		dest := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		relTarget, err := filepath.Rel(filepath.Dir(dest), abs)
		if err != nil {
			return err
		}
		_ = os.Remove(dest)
		return os.Symlink(relTarget, dest)
	})
}

// unlinkPkg removes the relative symlinks a package created under the prefix.
func unlinkPkg(prefix, name string) error {
	return walkPkgFiles(storePath(prefix, name), func(abs, rel string) error {
		dest := filepath.Join(prefix, rel)
		if fi, err := os.Lstat(dest); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(dest)
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// install (multi-package, three-phase) and supporting commands.
// ---------------------------------------------------------------------------

type installOpts struct {
	prefix string
	arch   string
	linked bool
	force  bool
}

// installTarget is one package's desired end state after installation.
type installTarget struct {
	name   string
	linked bool
}

// installPackages runs the three-phase multi-package install for packages that
// all share the same root-link setting.
func installPackages(names []string, o installOpts) error {
	plan := make([]installTarget, 0, len(names))
	for _, name := range names {
		plan = append(plan, installTarget{name: name, linked: o.linked})
	}
	return installPackagePlan(plan, o.prefix, o.arch, o.force)
}

// installPackagePlan runs the three-phase multi-package install with a
// per-package link plan. Downloads are still shared in one bounded-parallel
// batch; phase 3 then reconciles each package's final root-link state.
func installPackagePlan(plan []installTarget, prefix, arch string, force bool) error {
	start := time.Now()
	names := make([]string, len(plan))
	for i, pkg := range plan {
		names[i] = pkg.name
	}
	logger.Info("install started", "packages", names, "arch", arch,
		"prefix", prefix, "force", force)
	fmt.Printf("> Resolving %d package(s) for %s...\n", len(plan), arch)

	// Phase 1: resolve every package's layer digest in parallel. errgroup
	// collects the first error per goroutine; we gather *all* missing packages
	// so the user sees the complete list before anything is installed.
	phase1 := time.Now()
	digests := make([]string, len(plan))
	errs := make([]error, len(plan))
	var g errgroup.Group
	g.SetLimit(maxParallel)
	for i, pkg := range plan {
		i, pkg := i, pkg
		g.Go(func() error {
			d, err := remoteDigest(pkg.name, arch)
			digests[i], errs[i] = d, err
			if err != nil {
				logger.Error("resolve failed", "package", pkg.name, "arch", arch, "err", err)
			} else {
				logger.Debug("resolved digest", "package", pkg.name, "digest", d)
			}
			return nil
		})
	}
	_ = g.Wait()
	logger.Info("phase 1 (resolve) done", "count", len(plan), "took", time.Since(phase1).String())
	if joined := errors.Join(errs...); joined != nil {
		logger.Error("install aborted: unresolved packages", "err", joined)
		return fmt.Errorf("aborting, some packages could not be resolved:\n%w", joined)
	}

	// Decide which packages actually need downloading (skip up-to-date unless
	// --force).
	metas := make([]meta, len(plan))
	haveMeta := make([]bool, len(plan))
	toFetch := make([]bool, len(plan))
	var fetchNames []string
	var skipped int
	for i, pkg := range plan {
		if m, err := readMeta(prefix, pkg.name); err == nil {
			metas[i], haveMeta[i] = m, true
		}
		if !force && haveMeta[i] && metas[i].Digest == digests[i] {
				skipped++
			logger.Info("skip up-to-date", "package", pkg.name, "digest", digests[i])
			fmt.Printf("> %s (%s) is already up to date, skipping download. Use --force to reinstall.\n", pkg.name, arch)
			continue
		}
		toFetch[i] = true
		fetchNames = append(fetchNames, pkg.name)
	}

	// Phase 2: download the needed layers in parallel into the cache, each
	// with its own byte-level progress bar rendered by mpb.
	phase2 := time.Now()
	var dg errgroup.Group
	dg.SetLimit(maxParallel)
	if len(fetchNames) > 0 {
		fmt.Printf("> Downloading %d package(s)...\n", len(fetchNames))
		p := mpb.New(mpb.WithWidth(64))
		for _, name := range fetchNames {
			name := name
			dg.Go(func() error {
				logger.Debug("download started", "package", name)
				wrap := func(size int64, rc io.ReadCloser) io.ReadCloser {
					bar := p.New(size,
						mpb.BarStyle().Rbound("|"),
						mpb.PrependDecorators(
							decor.Name(name, decor.WC{C: decor.DindentRight | decor.DextraSpace, W: 22}),
							decor.CountersKibiByte("% .2f / % .2f"),
						),
						mpb.AppendDecorators(
							decor.Percentage(decor.WC{W: 5}),
							decor.Name(" "),
							decor.AverageSpeed(decor.SizeB1024(0), "% .2f"),
						),
					)
					return bar.ProxyReader(rc)
				}
				err := downloadLayer(name, arch, cachePath(arch, name), wrap)
				if err != nil {
					logger.Error("download failed", "package", name, "err", err)
				} else {
					logger.Debug("download done", "package", name)
				}
				return err
			})
		}
		werr := dg.Wait()
		p.Wait()
		if werr != nil {
			return fmt.Errorf("download failed: %w", werr)
		}
	}
	logger.Info("phase 2 (download) done", "count", len(fetchNames), "took", time.Since(phase2).String())

	// Phase 3: extract + reconcile root-link state serially.
	phase3 := time.Now()
	for i, pkg := range plan {
		store := storePath(prefix, pkg.name)
		currentLinked := haveMeta[i] && metas[i].Linked
		if currentLinked && (toFetch[i] || !pkg.linked) {
			if err := unlinkPkg(prefix, pkg.name); err != nil {
				logger.Error("unlink failed", "package", pkg.name, "err", err)
				return fmt.Errorf("%s: unlink failed: %w", pkg.name, err)
			}
		}
		if toFetch[i] {
			fmt.Printf("> Installing %s (%s) -> %s (linked=%t)\n", pkg.name, arch, store, pkg.linked)
			logger.Info("extract started", "package", pkg.name, "store", store, "linked", pkg.linked)
			if err := os.RemoveAll(store); err != nil {
				return err
			}
			if err := os.MkdirAll(store, 0o755); err != nil {
				return err
			}
			if err := extractTarGz(cachePath(arch, pkg.name), store); err != nil {
				logger.Error("extract failed", "package", pkg.name, "err", err)
				return fmt.Errorf("%s: extract failed: %w", pkg.name, err)
			}
		}
		if toFetch[i] || !haveMeta[i] || metas[i].Arch != arch || metas[i].Digest != digests[i] || metas[i].Linked != pkg.linked {
			if err := writeMeta(prefix, meta{Name: pkg.name, Arch: arch, Digest: digests[i], Linked: pkg.linked}); err != nil {
				return err
			}
		}
		if pkg.linked {
			if err := linkPkg(prefix, pkg.name); err != nil {
				logger.Error("link failed", "package", pkg.name, "err", err)
				return fmt.Errorf("%s: link failed: %w", pkg.name, err)
			}
		}
		if toFetch[i] {
			logger.Info("package installed", "package", pkg.name, "digest", digests[i])
			fmt.Printf("> Installed %s.\n", pkg.name)
		}
	}
	logger.Info("phase 3 (extract+reconcile) done", "count", len(plan), "took", time.Since(phase3).String())

	total := time.Since(start)
	logger.Info("install finished", "installed", len(fetchNames), "skipped", skipped, "took", total.String())
	fmt.Printf("> Done: %d installed, %d up-to-date in %s.\n", len(fetchNames), skipped, total.Round(time.Millisecond))

	return nil
}

func cmdRemove(prefix, name string) error {
	store := storePath(prefix, name)
	if _, err := os.Stat(store); err != nil {
		return fmt.Errorf("%s is not installed", name)
	}
	if m, err := readMeta(prefix, name); err == nil && m.Linked {
		if err := unlinkPkg(prefix, name); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(store); err != nil {
		return err
	}
	fmt.Printf("> Removed %s from %s.\n", name, prefix)
	return nil
}

// installedNames lists package names that have a metadata file under the store.
func installedNames(prefix string) []string {
	entries, err := os.ReadDir(filepath.Join(prefix, "store"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(metaPath(prefix, e.Name())); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func cmdList(prefix string) error {
	names := installedNames(prefix)
	if len(names) == 0 {
		fmt.Printf("No packages installed under %s.\n", prefix)
		return nil
	}
	fmt.Printf("%-22s %-15s %-7s %s\n", "NAME", "ARCH", "LINKED", "DIGEST")
	for _, name := range names {
		m, err := readMeta(prefix, name)
		if err != nil {
			continue
		}
		linked := "0"
		if m.Linked {
			linked = "1"
		}
		fmt.Printf("%-22s %-15s %-7s %s\n", m.Name, m.Arch, linked, short(m.Digest))
	}
	return nil
}

func cmdInfo(prefix, arch, name string) error {
	fmt.Printf("Package: %s\n", name)
	fmt.Printf("Registry: %s\n", ref(name, arch))
	remote, derr := remoteDigest(name, arch)
	if m, err := readMeta(prefix, name); err == nil {
		fmt.Printf("Status:  installed (%s)\n", storePath(prefix, name))
		fmt.Printf("  arch:    %s\n", m.Arch)
		fmt.Printf("  digest:  %s\n", m.Digest)
		fmt.Printf("  linked:  %t\n", m.Linked)
		fmt.Printf("  installed_at: %s\n", m.InstalledAt)
		switch {
		case derr != nil:
			fmt.Printf("  remote:  <error: %v>\n", derr)
		case m.Digest == remote:
			fmt.Printf("  remote:  %s (up to date)\n", remote)
		default:
			fmt.Printf("  remote:  %s (outdated)\n", remote)
		}
		return nil
	}
	fmt.Println("Status:  not installed")
	if derr != nil {
		return derr
	}
	fmt.Printf("  remote:  %s\n", remote)
	return nil
}

func cmdOutdated(prefix string) error {
	names := installedNames(prefix)
	if len(names) == 0 {
		fmt.Printf("No packages installed under %s.\n", prefix)
		return nil
	}
	any := false
	for _, name := range names {
		m, err := readMeta(prefix, name)
		if err != nil {
			continue
		}
		remote, err := remoteDigest(name, m.Arch)
		if err != nil {
			continue
		}
		if m.Digest != remote {
			any = true
			fmt.Printf("%-22s %s -> %s\n", name, short(m.Digest), short(remote))
		}
	}
	if !any {
		fmt.Println("All packages are up to date.")
	}
	return nil
}

func cmdUpgrade(prefix, arch string, names []string) error {
	if len(names) == 0 {
		names = installedNames(prefix)
		if len(names) == 0 {
			fmt.Printf("No packages installed under %s.\n", prefix)
			return nil
		}
	}
	for _, name := range names {
		m, err := readMeta(prefix, name)
		if err != nil {
			return fmt.Errorf("%s is not installed", name)
		}
		if err := installPackages([]string{name}, installOpts{
			prefix: prefix, arch: m.Arch, linked: m.Linked, force: false,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Declarative manifest (sb.yaml) and `sync`.
// ---------------------------------------------------------------------------

// defaultManifest is the manifest filename resolved in the current directory
// when `sync` is invoked without an explicit path.
const defaultManifest = "sb.yaml"

// manifest is a declarative set of packages to install. It is the sb
// equivalent of a Brewfile / pyproject file.
//
//   - packages.link:   installed into the store and linked into the prefix root.
//   - packages.unlink: installed into the store only (not linked into root).
//   - profiles:        a sync-only convenience — each named profile installs its
//     packages into the store and aggregates their files via relative symlinks
//     under <prefix>/profile/<name>/ (profile packages are not linked into the
//     prefix root).
//
// The rest of the client (install/remove/list) is unaware of profiles / the
// link split; they exist purely in the manifest/sync layer.
type manifest struct {
	Prefix   string              `yaml:"prefix"`
	Arch     string              `yaml:"arch"`
	Packages packageSet          `yaml:"packages"`
	Profiles map[string][]string `yaml:"profiles"`
}

// packageSet splits a manifest's packages by whether they are exposed via
// symlinks in the prefix root.
type packageSet struct {
	Link   []string `yaml:"link"`
	Unlink []string `yaml:"unlink"`
}

// loadManifest reads and parses a YAML manifest, requiring at least one package
// (in packages.link, packages.unlink, or a profile).
func loadManifest(path string) (manifest, error) {
	var m manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("%s: %w", path, err)
	}
	if len(m.Packages.Link) == 0 && len(m.Packages.Unlink) == 0 && len(m.Profiles) == 0 {
		return m, fmt.Errorf("%s: no packages listed", path)
	}
	return m, nil
}

// profilePackages returns the sorted, de-duplicated set of package names
// referenced by all profiles.
func (m manifest) profilePackages() []string {
	seen := make(map[string]bool)
	for _, pkgs := range m.Profiles {
		for _, name := range pkgs {
			seen[name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// installPlan returns the ordered, de-duplicated install set for a sync run.
// Root-linked packages win if a name is referenced in multiple manifest
// sections; profile packages are included only once in the shared install plan.
func (m manifest) installPlan(profilePkgs []string) []installTarget {
	var plan []installTarget
	seen := make(map[string]int)
	add := func(name string, linked bool) {
		if idx, ok := seen[name]; ok {
			if linked {
				plan[idx].linked = true
			}
			return
		}
		seen[name] = len(plan)
		plan = append(plan, installTarget{name: name, linked: linked})
	}
	for _, name := range m.Packages.Link {
		add(name, true)
	}
	for _, name := range m.Packages.Unlink {
		add(name, false)
	}
	for _, name := range profilePkgs {
		add(name, false)
	}
	return plan
}

// cmdSync reconciles the installed set against a manifest. It installs/refreshes
// every distinct listed package in one shared batch, then applies the desired
// root/profile links, and when prune is set removes installed packages that the
// manifest no longer references. The manifest's prefix/arch (if set) are used
// unless overridden: an explicit --arch always wins for arch; the manifest
// prefix is used only when --prefix was not passed explicitly (prefixSet is
// false). Note the log file still lives under the flag's prefix.
func cmdSync(prefix, arch, file string, prefixSet, force, prune bool) error {
	m, err := loadManifest(file)
	if err != nil {
		return err
	}
	if m.Arch != "" {
		arch = m.Arch
	}
	if !prefixSet && m.Prefix != "" {
		prefix = m.Prefix
	}
	profilePkgs := m.profilePackages()
	plan := m.installPlan(profilePkgs)
	logger.Info("sync started", "file", file, "prefix", prefix, "link", m.Packages.Link,
		"unlink", m.Packages.Unlink, "profiles", len(m.Profiles), "prune", prune)
	fmt.Printf("> Syncing %d unique package(s)", len(plan))
	if len(m.Profiles) > 0 {
		fmt.Printf(" including %d unique profile package(s)", len(profilePkgs))
	}
	fmt.Printf(" from %s...\n", file)

	// Unlinked packages and profile packages still land in the store only; the
	// shared install plan simply lets sync resolve/download them once.
	if len(plan) > 0 {
		if err := installPackagePlan(plan, prefix, arch, force); err != nil {
			return err
		}
	}
	for _, profile := range sortedKeys(m.Profiles) {
		root := filepath.Join(prefix, "profile", profile)
		fmt.Printf("> Linking profile %s -> %s\n", profile, root)
		for _, name := range m.Profiles[profile] {
			logger.Info("profile link", "profile", profile, "package", name, "root", root)
			if err := linkPkgInto(prefix, name, root); err != nil {
				return fmt.Errorf("profile %s: %s: %w", profile, name, err)
			}
		}
	}

	if !prune {
		return nil
	}
	want := make(map[string]bool, len(plan))
	for _, pkg := range plan {
		want[pkg.name] = true
	}
	for _, name := range installedNames(prefix) {
		if want[name] {
			continue
		}
		logger.Info("prune removing package not in manifest", "package", name)
		if err := cmdRemove(prefix, name); err != nil {
			return err
		}
	}
	return nil
}

// sortedKeys returns the map keys in deterministic (sorted) order.
func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func short(digest string) string {
	if len(digest) > 19 {
		return digest[:19]
	}
	return digest
}

// ---------------------------------------------------------------------------
// CLI (cobra).
// ---------------------------------------------------------------------------

func main() {
	var (
		prefix  string
		arch    string
		link    bool
		force   bool
		verbose bool
	)
	var logCloser io.Closer

	// resolveArch returns the explicit --arch or the auto-detected platform.
	resolveArch := func() (string, error) {
		if arch != "" {
			return arch, nil
		}
		return detectArch()
	}

	root := &cobra.Command{
		Use:           "sb",
		Short:         "package manager for ghcr.io/curoky/standalone-binaries",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := setupLogger(prefix, verbose)
			if err != nil {
				return err
			}
			logCloser = c
			logger.Info("sb invoked", "command", cmd.Name(), "args", args,
				"prefix", prefix, "arch", arch, "verbose", verbose)
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if logCloser != nil {
				_ = logCloser.Close()
			}
		},
	}
	pf := root.PersistentFlags()
	pf.StringVar(&prefix, "prefix", defaultPrefix, "install prefix")
	pf.StringVar(&arch, "arch", "", "arch tag: linux-x86_64 | darwin-arm64 (auto-detected)")
	pf.BoolVar(&verbose, "verbose", false, "also print the detailed log to stderr")

	install := &cobra.Command{
		Use:   "install <package>...",
		Short: "Install/refresh one or more packages",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := resolveArch()
			if err != nil {
				return err
			}
			return installPackages(args, installOpts{prefix: prefix, arch: a, linked: link, force: force})
		},
	}
	install.Flags().BoolVar(&link, "link", true, "expose binaries via relative symlinks")
	install.Flags().BoolVar(&force, "force", false, "reinstall even if the digest already matches")

	remove := &cobra.Command{
		Use:   "remove <package>",
		Short: "Uninstall a package and clean up its links",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return cmdRemove(prefix, args[0]) },
	}

	upgrade := &cobra.Command{
		Use:   "upgrade [package...]",
		Short: "Upgrade the given packages, or all installed packages if none is given",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := resolveArch()
			if err != nil {
				return err
			}
			return cmdUpgrade(prefix, a, args)
		},
	}

	info := &cobra.Command{
		Use:   "info <package>",
		Short: "Show a package's metadata and whether it is up to date",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := resolveArch()
			if err != nil {
				return err
			}
			return cmdInfo(prefix, a, args[0])
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List installed packages and their recorded digests",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmdList(prefix) },
	}

	outdated := &cobra.Command{
		Use:   "outdated",
		Short: "Show installed packages whose remote digest has changed",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmdOutdated(prefix) },
	}

	var prune bool
	sync := &cobra.Command{
		Use:   "sync [file]",
		Short: "Install packages declared in a YAML manifest (default: sb.yaml)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := resolveArch()
			if err != nil {
				return err
			}
			file := defaultManifest
			if len(args) == 1 {
				file = args[0]
			}
			return cmdSync(prefix, a, file, cmd.Flags().Changed("prefix"), force, prune)
		},
	}
	sync.Flags().BoolVar(&force, "force", false, "reinstall even if the digest already matches")
	sync.Flags().BoolVar(&prune, "prune", false, "remove installed packages not listed in the manifest")

	root.AddCommand(install, remove, upgrade, info, list, outdated, sync)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
