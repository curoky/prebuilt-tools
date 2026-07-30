package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultManifest = "binman.yaml"

type manifest struct {
	Prefix   string              `yaml:"prefix"`
	Arch     string              `yaml:"arch"`
	Packages packageSet          `yaml:"packages"`
	Profiles map[string][]string `yaml:"profiles"`
}

type packageSet struct {
	Link   []string `yaml:"link"`
	Unlink []string `yaml:"unlink"`
}

func loadManifest(path string) (manifest, error) {
	var config manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return config, fmt.Errorf("%s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return config, fmt.Errorf("%s: multiple YAML documents are not supported", path)
		}
		return config, fmt.Errorf("%s: %w", path, err)
	}
	if config.Arch != "" {
		if err := validateArch(config.Arch); err != nil {
			return config, fmt.Errorf("%s: %w", path, err)
		}
	}
	count := 0
	for _, packageName := range append(append([]string{}, config.Packages.Link...), config.Packages.Unlink...) {
		if err := validatePackageName(packageName); err != nil {
			return config, fmt.Errorf("%s: %w", path, err)
		}
		count++
	}
	for profile, packages := range config.Profiles {
		if err := validateProfileName(profile); err != nil {
			return config, fmt.Errorf("%s: %w", path, err)
		}
		for _, packageName := range packages {
			if err := validatePackageName(packageName); err != nil {
				return config, fmt.Errorf("%s: profile %s: %w", path, profile, err)
			}
			count++
		}
	}
	if count == 0 {
		return config, fmt.Errorf("%s: no packages listed", path)
	}
	return config, nil
}

func (config manifest) profilePackages() []string {
	seen := make(map[string]bool)
	for _, packages := range config.Profiles {
		for _, packageName := range packages {
			seen[packageName] = true
		}
	}
	names := make([]string, 0, len(seen))
	for packageName := range seen {
		names = append(names, packageName)
	}
	sort.Strings(names)
	return names
}

// installPlan de-duplicates packages while preserving manifest order. A root
// link wins when a package appears in multiple sections.
func (config manifest) installPlan(profilePackages []string) []installTarget {
	var plan []installTarget
	seen := make(map[string]int)
	add := func(packageName string, linked bool) {
		if index, ok := seen[packageName]; ok {
			if linked {
				plan[index].linked = true
			}
			return
		}
		seen[packageName] = len(plan)
		plan = append(plan, installTarget{name: packageName, linked: linked})
	}
	for _, packageName := range config.Packages.Link {
		add(packageName, true)
	}
	for _, packageName := range config.Packages.Unlink {
		add(packageName, false)
	}
	for _, packageName := range profilePackages {
		add(packageName, false)
	}
	return plan
}

func cmdSync(prefix, arch, file string, prefixSet, archSet, force, prune bool) error {
	config, err := loadManifest(file)
	if err != nil {
		return err
	}
	if !archSet && config.Arch != "" {
		arch = config.Arch
	}
	if !prefixSet && config.Prefix != "" {
		prefix = config.Prefix
	}
	profilePackages := config.profilePackages()
	plan := config.installPlan(profilePackages)
	logger.Info("sync started", "file", file, "prefix", prefix, "link", config.Packages.Link,
		"unlink", config.Packages.Unlink, "profiles", len(config.Profiles), "prune", prune)
	fmt.Printf("> Syncing %d unique package(s)", len(plan))
	if len(config.Profiles) > 0 {
		fmt.Printf(" including %d unique profile package(s)", len(profilePackages))
	}
	fmt.Printf(" from %s...\n", file)

	if len(plan) > 0 {
		if err := installPackagePlan(plan, prefix, arch, force); err != nil {
			return err
		}
	}
	profileParent := filepath.Join(prefix, "profile")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return err
	}
	stagedProfiles, err := os.MkdirTemp(prefix, ".profile.stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stagedProfiles)
	for _, profile := range sortedProfileNames(config.Profiles) {
		root := filepath.Join(stagedProfiles, profile)
		fmt.Printf("> Linking profile %s -> %s\n", profile, filepath.Join(profileParent, profile))
		for _, packageName := range config.Profiles[profile] {
			logger.Info("profile link", "profile", profile, "package", packageName, "root", root)
			if err := linkPkgInto(prefix, packageName, root); err != nil {
				return fmt.Errorf("profile %s: %s: %w", profile, packageName, err)
			}
		}
	}
	if err := replaceTree(profileParent, stagedProfiles); err != nil {
		return fmt.Errorf("replace profiles: %w", err)
	}

	if !prune {
		return nil
	}
	wanted := make(map[string]bool, len(plan))
	for _, target := range plan {
		wanted[target.name] = true
	}
	installed, err := installedNames(prefix)
	if err != nil {
		return err
	}
	for _, packageName := range installed {
		if wanted[packageName] {
			continue
		}
		logger.Info("prune removing package not in manifest", "package", packageName)
		if err := cmdRemove(prefix, packageName); err != nil {
			return err
		}
	}
	return nil
}

func replaceTree(dst, staged string) error {
	backup := ""
	if _, err := os.Stat(dst); err == nil {
		backupDir, err := os.MkdirTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".backup-")
		if err != nil {
			return err
		}
		if err := os.Remove(backupDir); err != nil {
			return err
		}
		backup = backupDir
		if err := os.Rename(dst, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(staged, dst); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dst)
		}
		return err
	}
	if backup != "" {
		return os.RemoveAll(backup)
	}
	return nil
}

func sortedProfileNames(profiles map[string][]string) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
