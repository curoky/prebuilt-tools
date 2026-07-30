package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/sync/errgroup"
)

type installOpts struct {
	prefix string
	arch   string
	linked bool
	force  bool
}

type installTarget struct {
	name   string
	arch   string
	linked bool
}

func installPackages(names []string, options installOpts) error {
	plan := make([]installTarget, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, packageName := range names {
		if err := validatePackageName(packageName); err != nil {
			return err
		}
		if seen[packageName] {
			continue
		}
		seen[packageName] = true
		plan = append(plan, installTarget{
			name: packageName, arch: options.arch, linked: options.linked,
		})
	}
	return installPackagePlan(plan, options.prefix, options.arch, options.force)
}

// installPackagePlan resolves and downloads in bounded parallel batches, then
// commits packages serially because package links share the same prefix.
func installPackagePlan(plan []installTarget, prefix, arch string, force bool) error {
	if arch != "" {
		if err := validateArch(arch); err != nil {
			return err
		}
	}
	for index := range plan {
		if err := validatePackageName(plan[index].name); err != nil {
			return err
		}
		if plan[index].arch == "" {
			plan[index].arch = arch
		}
		if err := validateArch(plan[index].arch); err != nil {
			return fmt.Errorf("%s: %w", plan[index].name, err)
		}
	}

	start := time.Now()
	names := make([]string, len(plan))
	for index, target := range plan {
		names[index] = target.name
	}
	logger.Info("install started", "packages", names, "arch", arch,
		"prefix", prefix, "force", force)
	fmt.Printf("> Resolving %d package(s)...\n", len(plan))

	phase1 := time.Now()
	digests := make([]string, len(plan))
	layers := make([]v1.Layer, len(plan))
	resolveErrors := make([]error, len(plan))
	var resolveGroup errgroup.Group
	resolveGroup.SetLimit(maxParallel)
	for index, target := range plan {
		index, target := index, target
		resolveGroup.Go(func() error {
			layer, err := remoteLayer(target.name, target.arch)
			if err == nil {
				digest, digestErr := layer.Digest()
				if digestErr != nil {
					err = fmt.Errorf("%s: %w", target.name, digestErr)
				} else {
					digests[index] = digest.String()
					layers[index] = layer
				}
			}
			resolveErrors[index] = err
			if err != nil {
				logger.Error("resolve failed", "package", target.name, "arch", target.arch, "err", err)
			} else {
				logger.Debug("resolved digest", "package", target.name, "digest", digests[index])
			}
			return nil
		})
	}
	_ = resolveGroup.Wait()
	logger.Info("phase 1 (resolve) done", "count", len(plan), "took", time.Since(phase1).String())
	if err := errors.Join(resolveErrors...); err != nil {
		logger.Error("install aborted: unresolved packages", "err", err)
		return fmt.Errorf("aborting, some packages could not be resolved:\n%w", err)
	}

	metas := make([]meta, len(plan))
	haveMeta := make([]bool, len(plan))
	toFetch := make([]bool, len(plan))
	fetchCount := 0
	skipped := 0
	for index, target := range plan {
		if metadata, err := readMeta(prefix, target.name); err == nil {
			metas[index], haveMeta[index] = metadata, true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("%s: read metadata: %w", target.name, err)
		} else if _, statErr := os.Stat(storePath(prefix, target.name)); statErr == nil {
			return fmt.Errorf("%s: store exists without valid metadata", target.name)
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		if !force && haveMeta[index] && metas[index].Digest == digests[index] {
			skipped++
			logger.Info("skip up-to-date", "package", target.name, "digest", digests[index])
			fmt.Printf("> %s (%s) is already up to date, skipping download. Use --force to reinstall.\n",
				target.name, target.arch)
			continue
		}
		toFetch[index] = true
		fetchCount++
	}

	phase2 := time.Now()
	var downloadGroup errgroup.Group
	downloadGroup.SetLimit(maxParallel)
	if fetchCount > 0 {
		fmt.Printf("> Downloading %d package(s)...\n", fetchCount)
		progress := mpb.New(mpb.WithWidth(64))
		for index, target := range plan {
			if !toFetch[index] {
				continue
			}
			index, target := index, target
			downloadGroup.Go(func() error {
				logger.Debug("download started", "package", target.name)
				var bar *mpb.Bar
				wrap := func(size int64, reader io.ReadCloser) io.ReadCloser {
					bar = progress.New(size,
						mpb.BarStyle().Rbound("|"),
						mpb.PrependDecorators(
							decor.Name(target.name, decor.WC{
								C: decor.DindentRight | decor.DextraSpace, W: 22,
							}),
							decor.CountersKibiByte("% .2f / % .2f"),
						),
						mpb.AppendDecorators(
							decor.Percentage(decor.WC{W: 5}),
							decor.Name(" "),
							decor.AverageSpeed(decor.SizeB1024(0), "% .2f"),
						),
					)
					return bar.ProxyReader(reader)
				}
				err := downloadLayer(layers[index], cachePath(target.arch, target.name), wrap)
				if err != nil {
					if bar != nil {
						bar.Abort(false)
					}
					logger.Error("download failed", "package", target.name, "err", err)
				} else {
					logger.Debug("download done", "package", target.name)
				}
				return err
			})
		}
		err := downloadGroup.Wait()
		progress.Wait()
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
	}
	logger.Info("phase 2 (download) done", "count", fetchCount, "took", time.Since(phase2).String())

	phase3 := time.Now()
	for index, target := range plan {
		store := storePath(prefix, target.name)
		currentLinked := haveMeta[index] && metas[index].Linked
		if toFetch[index] {
			fmt.Printf("> Installing %s (%s) -> %s (linked=%t)\n",
				target.name, target.arch, store, target.linked)
			logger.Info("extract started", "package", target.name, "store", store, "linked", target.linked)
			parent := filepath.Dir(store)
			if err := os.MkdirAll(parent, 0o755); err != nil {
				return err
			}
			staged, err := os.MkdirTemp(parent, "."+target.name+".stage-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(staged)
			if err := extractTarGz(cachePath(target.arch, target.name), staged); err != nil {
				logger.Error("extract failed", "package", target.name, "err", err)
				return fmt.Errorf("%s: extract failed: %w", target.name, err)
			}
			nextMeta := meta{
				Name: target.name, Arch: target.arch, Digest: digests[index], Linked: target.linked,
			}
			if err := writeMetaAt(staged, nextMeta); err != nil {
				return err
			}
			if err := replaceStore(prefix, target.name, staged, currentLinked, target.linked); err != nil {
				return err
			}
		} else {
			linkStateChanged := currentLinked != target.linked
			if currentLinked && !target.linked {
				if err := unlinkPkg(prefix, target.name); err != nil {
					logger.Error("unlink failed", "package", target.name, "err", err)
					return fmt.Errorf("%s: unlink failed: %w", target.name, err)
				}
			}
			if target.linked {
				if err := linkPkg(prefix, target.name); err != nil {
					logger.Error("link failed", "package", target.name, "err", err)
					return fmt.Errorf("%s: link failed: %w", target.name, err)
				}
			}
			if !haveMeta[index] || metas[index].Arch != target.arch ||
				metas[index].Digest != digests[index] || metas[index].Linked != target.linked {
				if err := writeMeta(prefix, meta{
					Name: target.name, Arch: target.arch, Digest: digests[index], Linked: target.linked,
				}); err != nil {
					if linkStateChanged {
						if target.linked {
							_ = unlinkPkg(prefix, target.name)
						} else {
							_ = linkPkg(prefix, target.name)
						}
					}
					return err
				}
			}
		}
		if toFetch[index] {
			logger.Info("package installed", "package", target.name, "digest", digests[index])
			fmt.Printf("> Installed %s.\n", target.name)
		}
	}
	logger.Info("phase 3 (extract+reconcile) done", "count", len(plan), "took", time.Since(phase3).String())

	total := time.Since(start)
	logger.Info("install finished", "installed", fetchCount, "skipped", skipped, "took", total.String())
	fmt.Printf("> Done: %d installed, %d up-to-date in %s.\n",
		fetchCount, skipped, total.Round(time.Millisecond))
	return nil
}

func cmdRemove(prefix, packageName string) error {
	if err := validatePackageName(packageName); err != nil {
		return err
	}
	store := storePath(prefix, packageName)
	if _, err := os.Stat(store); os.IsNotExist(err) {
		return fmt.Errorf("%s is not installed", packageName)
	} else if err != nil {
		return err
	}
	metadata, err := readMeta(prefix, packageName)
	if err != nil {
		return fmt.Errorf("%s: read metadata: %w", packageName, err)
	}
	if metadata.Linked {
		if err := unlinkPkg(prefix, packageName); err != nil {
			return err
		}
	}
	profiles, err := os.ReadDir(filepath.Join(prefix, "profile"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, profile := range profiles {
		if profile.IsDir() {
			if err := unlinkPkgFrom(prefix, packageName,
				filepath.Join(prefix, "profile", profile.Name())); err != nil {
				return err
			}
		}
	}
	if err := os.RemoveAll(store); err != nil {
		return err
	}
	fmt.Printf("> Removed %s from %s.\n", packageName, prefix)
	return nil
}

func installedNames(prefix string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(prefix, "store"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(metaPath(prefix, entry.Name())); err == nil {
			names = append(names, entry.Name())
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Strings(names)
	return names, nil
}

func cmdList(prefix string) error {
	names, err := installedNames(prefix)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Printf("No packages installed under %s.\n", prefix)
		return nil
	}
	fmt.Printf("%-22s %-15s %-7s %s\n", "NAME", "ARCH", "LINKED", "DIGEST")
	for _, packageName := range names {
		metadata, err := readMeta(prefix, packageName)
		if err != nil {
			return fmt.Errorf("%s: read metadata: %w", packageName, err)
		}
		linked := "0"
		if metadata.Linked {
			linked = "1"
		}
		fmt.Printf("%-22s %-15s %-7s %s\n",
			metadata.Name, metadata.Arch, linked, short(metadata.Digest))
	}
	return nil
}

func cmdInfo(prefix, arch, packageName string) error {
	if err := validatePackageName(packageName); err != nil {
		return err
	}
	if err := validateArch(arch); err != nil {
		return err
	}
	metadata, metadataError := readMeta(prefix, packageName)
	if metadataError != nil && !os.IsNotExist(metadataError) {
		return fmt.Errorf("%s: read metadata: %w", packageName, metadataError)
	}
	fmt.Printf("Package: %s\n", packageName)
	fmt.Printf("Registry: %s\n", ref(packageName, arch))
	remote, remoteError := remoteDigest(packageName, arch)
	if metadataError == nil {
		fmt.Printf("Status:  installed (%s)\n", storePath(prefix, packageName))
		fmt.Printf("  arch:    %s\n", metadata.Arch)
		fmt.Printf("  digest:  %s\n", metadata.Digest)
		fmt.Printf("  linked:  %t\n", metadata.Linked)
		fmt.Printf("  installed_at: %s\n", metadata.InstalledAt)
		switch {
		case remoteError != nil:
			fmt.Printf("  remote:  <error: %v>\n", remoteError)
		case metadata.Digest == remote:
			fmt.Printf("  remote:  %s (up to date)\n", remote)
		default:
			fmt.Printf("  remote:  %s (outdated)\n", remote)
		}
		return nil
	}
	fmt.Println("Status:  not installed")
	if remoteError != nil {
		return remoteError
	}
	fmt.Printf("  remote:  %s\n", remote)
	return nil
}

func cmdOutdated(prefix string) error {
	names, err := installedNames(prefix)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Printf("No packages installed under %s.\n", prefix)
		return nil
	}
	metas := make([]meta, len(names))
	remotes := make([]string, len(names))
	resolveErrors := make([]error, len(names))
	var group errgroup.Group
	group.SetLimit(maxParallel)
	for index, packageName := range names {
		metadata, err := readMeta(prefix, packageName)
		if err != nil {
			resolveErrors[index] = fmt.Errorf("%s: read metadata: %w", packageName, err)
			continue
		}
		metas[index] = metadata
		index, packageName := index, packageName
		group.Go(func() error {
			remotes[index], resolveErrors[index] = remoteDigest(packageName, metadata.Arch)
			return nil
		})
	}
	_ = group.Wait()
	if err := errors.Join(resolveErrors...); err != nil {
		return fmt.Errorf("could not check all packages:\n%w", err)
	}

	any := false
	for index, packageName := range names {
		if metas[index].Digest != remotes[index] {
			any = true
			fmt.Printf("%-22s %s -> %s\n",
				packageName, short(metas[index].Digest), short(remotes[index]))
		}
	}
	if !any {
		fmt.Println("All packages are up to date.")
	}
	return nil
}

func cmdUpgrade(prefix, arch string, names []string) error {
	if len(names) == 0 {
		var err error
		names, err = installedNames(prefix)
		if err != nil {
			return err
		}
		if len(names) == 0 {
			fmt.Printf("No packages installed under %s.\n", prefix)
			return nil
		}
	}
	plan := make([]installTarget, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, packageName := range names {
		if err := validatePackageName(packageName); err != nil {
			return err
		}
		if seen[packageName] {
			continue
		}
		seen[packageName] = true
		metadata, err := readMeta(prefix, packageName)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("%s is not installed", packageName)
			}
			return fmt.Errorf("%s: read metadata: %w", packageName, err)
		}
		targetArch := metadata.Arch
		if arch != "" {
			targetArch = arch
		}
		plan = append(plan, installTarget{
			name: packageName, arch: targetArch, linked: metadata.Linked,
		})
	}
	return installPackagePlan(plan, prefix, arch, false)
}

func short(digest string) string {
	if len(digest) > 19 {
		return digest[:19]
	}
	return digest
}
