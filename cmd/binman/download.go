package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// downloadPackages fetches package archives and extracts each one into a
// package-named directory without creating binman installation state.
func downloadPackages(names []string, arch, destination string) error {
	if err := validateArch(arch); err != nil {
		return err
	}
	absoluteDestination, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve output directory: %w", err)
	}
	info, err := os.Stat(absoluteDestination)
	if err != nil {
		return fmt.Errorf("output directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path is not a directory: %s", absoluteDestination)
	}

	requests := make([]artifactRequest, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		if err := validatePackageName(name); err != nil {
			return err
		}
		target := filepath.Join(absoluteDestination, name)
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("refusing to overwrite existing path: %s", target)
		} else if !os.IsNotExist(err) {
			return err
		}
		seen[name] = true
		requests = append(requests, artifactRequest{name: name, arch: arch})
	}

	fmt.Printf("> Resolving %d package(s)...\n", len(requests))
	artifacts, err := resolveArtifacts(requests)
	if err != nil {
		return fmt.Errorf("aborting, some packages could not be resolved:\n%w", err)
	}

	workspace, err := os.MkdirTemp(absoluteDestination, ".binman-download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)

	fmt.Printf("> Downloading %d package(s)...\n", len(artifacts))
	downloads := make([]artifactDownload, len(artifacts))
	for index := range artifacts {
		downloads[index] = artifactDownload{
			packageArtifact: artifacts[index],
			destination:     filepath.Join(workspace, artifacts[index].name+".tar.gz"),
		}
	}
	if err := downloadArtifacts(downloads); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	for _, artifact := range artifacts {
		stage := filepath.Join(workspace, artifact.name)
		if err := os.Mkdir(stage, 0o755); err != nil {
			return err
		}
		archive := filepath.Join(workspace, artifact.name+".tar.gz")
		if err := extractTarGz(archive, stage); err != nil {
			return fmt.Errorf("%s: extract failed: %w", artifact.name, err)
		}
	}

	for _, artifact := range artifacts {
		stage := filepath.Join(workspace, artifact.name)
		target := filepath.Join(absoluteDestination, artifact.name)
		if err := os.Rename(stage, target); err != nil {
			return fmt.Errorf("%s: place downloaded package: %w", artifact.name, err)
		}
		fmt.Printf("> Downloaded %s (%s) -> %s\n", artifact.name, arch, target)
	}
	return nil
}
