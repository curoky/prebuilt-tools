package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
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

type installJob struct {
	packageArtifact
	linked  bool
	current *meta
	fetch   bool
}

func installPackages(names []string, options installOpts) error {
	plan := make([]installTarget, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, packageName := range names {
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
	logger.Info("install started", "count", len(plan), "prefix", prefix, "force", force)
	fmt.Printf("> Resolving %d package(s)...\n", len(plan))

	requests := make([]artifactRequest, len(plan))
	for index := range plan {
		requests[index] = artifactRequest{name: plan[index].name, arch: plan[index].arch}
	}
	artifacts, err := resolveArtifacts(requests)
	if err != nil {
		logger.Error("install aborted: unresolved packages", "err", err)
		return fmt.Errorf("aborting, some packages could not be resolved:\n%w", err)
	}
	jobs := make([]installJob, len(artifacts))
	for index := range artifacts {
		jobs[index] = installJob{
			packageArtifact: artifacts[index],
			linked:          plan[index].linked,
		}
	}

	fetchCount := 0
	skipped := 0
	for index := range jobs {
		job := &jobs[index]
		if metadata, err := readMeta(prefix, job.name); err == nil {
			job.current = &metadata
		} else if _, statErr := os.Stat(storePath(prefix, job.name)); statErr == nil {
			// A store dir without valid metadata (missing or corrupt) is a
			// leftover from an interrupted install; drop it so this run
			// reinstalls cleanly.
			if err := os.RemoveAll(storePath(prefix, job.name)); err != nil {
				return fmt.Errorf("%s: clean leftover store: %w", job.name, err)
			}
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		if !force && job.current != nil && job.current.Digest == job.digest {
			skipped++
			fmt.Printf("> %s (%s) is already up to date, skipping download. Use --force to reinstall.\n",
				job.name, job.arch)
			continue
		}
		job.fetch = true
		fetchCount++
	}

	if fetchCount > 0 {
		fmt.Printf("> Downloading %d package(s)...\n", fetchCount)
		downloads := make([]artifactDownload, 0, fetchCount)
		for index := range jobs {
			job := &jobs[index]
			if !job.fetch {
				continue
			}
			downloads = append(downloads, artifactDownload{
				packageArtifact: job.packageArtifact,
				destination:     cachePath(job.arch, job.name),
			})
		}
		if err := downloadArtifacts(downloads); err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
	}

	for index := range jobs {
		job := &jobs[index]
		store := storePath(prefix, job.name)
		currentLinked := job.current != nil && job.current.Linked
		if job.fetch {
			fmt.Printf("> Installing %s (%s) -> %s (linked=%t)\n",
				job.name, job.arch, store, job.linked)
			parent := filepath.Dir(store)
			if err := os.MkdirAll(parent, 0o755); err != nil {
				return err
			}
			staged, err := os.MkdirTemp(parent, "."+job.name+".stage-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(staged)
			if err := extractTarGz(cachePath(job.arch, job.name), staged); err != nil {
				return fmt.Errorf("%s: extract failed: %w", job.name, err)
			}
			nextMeta := meta{
				Name: job.name, Arch: job.arch, Digest: job.digest, Linked: job.linked,
			}
			if err := writeMetaAt(staged, nextMeta); err != nil {
				return err
			}
			if err := placeStore(prefix, job.name, staged, currentLinked, job.linked); err != nil {
				return err
			}
		} else {
			if currentLinked && !job.linked {
				if err := unlinkPkg(prefix, job.name); err != nil {
					return fmt.Errorf("%s: unlink failed: %w", job.name, err)
				}
			}
			if job.linked {
				if err := linkPkg(prefix, job.name); err != nil {
					return fmt.Errorf("%s: link failed: %w", job.name, err)
				}
			}
			if job.current.Arch != job.arch || job.current.Linked != job.linked {
				if err := writeMeta(prefix, meta{
					Name: job.name, Arch: job.arch, Digest: job.digest, Linked: job.linked,
				}); err != nil {
					return err
				}
			}
		}
		if job.fetch {
			fmt.Printf("> Installed %s.\n", job.name)
		}
	}

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
	requests := make([]artifactRequest, len(names))
	resolveErrors := make([]error, len(names))
	for index, packageName := range names {
		metadata, err := readMeta(prefix, packageName)
		if err != nil {
			resolveErrors[index] = fmt.Errorf("%s: read metadata: %w", packageName, err)
			continue
		}
		metas[index] = metadata
		requests[index] = artifactRequest{name: packageName, arch: metadata.Arch}
	}
	if err := errors.Join(resolveErrors...); err != nil {
		return fmt.Errorf("could not check all packages:\n%w", err)
	}
	artifacts, err := resolveArtifacts(requests)
	if err != nil {
		return fmt.Errorf("could not check all packages:\n%w", err)
	}

	any := false
	for index, packageName := range names {
		if metas[index].Digest != artifacts[index].digest {
			any = true
			fmt.Printf("%-22s %s -> %s\n",
				packageName, short(metas[index].Digest), short(artifacts[index].digest))
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
