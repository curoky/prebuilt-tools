package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	storeShebangPattern   = regexp.MustCompile(`(?m)^#![[:space:]]*/nix/store/[a-z0-9._-]+/bin/([^[:space:]]+)`)
	storeBinPattern       = regexp.MustCompile(`/nix/store/[a-z0-9._-]+/bin/`)
	storePathPattern      = regexp.MustCompile(`/nix/store/[a-z0-9]{32}-[^[:space:]:/()<>]*`)
	storeReferencePattern = regexp.MustCompile(
		`/nix/store/[a-z0-9]{32}-`,
	)
)

func normalize(cfg config) error {
	if err := copyTree(cfg.source, cfg.output); err != nil {
		return fmt.Errorf("copy package tree: %w", err)
	}
	for _, relativePath := range []string{
		"nix-support",
		"share/man",
		"share/doc",
		"share/bash-completion",
	} {
		if err := os.RemoveAll(filepath.Join(cfg.output, relativePath)); err != nil {
			return fmt.Errorf("remove %s: %w", relativePath, err)
		}
	}

	files, err := regularFiles(cfg.output)
	if err != nil {
		return err
	}
	for _, path := range files {
		if err := prepareFile(path); err != nil {
			return err
		}
	}
	if err := removeDanglingSymlinks(cfg.output); err != nil {
		return err
	}
	files, err = regularFiles(cfg.output)
	if err != nil {
		return err
	}
	for _, path := range files {
		if err := normalizeFile(path, cfg); err != nil {
			return err
		}
	}
	if err := normalizeModes(cfg.output); err != nil {
		return fmt.Errorf("normalize modes: %w", err)
	}
	return nil
}

func removeDanglingSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		if _, err := filepath.EvalSymlinks(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return os.Remove(path)
			}
			return fmt.Errorf("resolve symlink %s: %w", path, err)
		}
		return nil
	})
}

func regularFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	return files, nil
}

func prepareFile(path string) error {
	if strings.HasSuffix(path, ".a") || strings.HasSuffix(path, ".pyc") {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}

	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") && strings.HasSuffix(base, "-wrapped") {
		publicName := strings.TrimSuffix(strings.TrimPrefix(base, "."), "-wrapped")
		target := filepath.Join(filepath.Dir(path), publicName)
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("replace wrapped entry %s: %w", target, err)
		}
		if err := os.Rename(path, target); err != nil {
			return fmt.Errorf("rename wrapped entry %s: %w", path, err)
		}
	}
	return nil
}

func normalizeFile(path string, cfg config) error {
	magic, err := fileMagic(path)
	if err != nil {
		return err
	}
	switch {
	case isELFMagic(magic):
		if cfg.platform == platformLinux {
			stripBinary(path)
		}
		if err := nukeStoreReferences(path); err != nil {
			return err
		}
		if cfg.platform == platformLinux && !strings.Contains(path, "openssl") {
			if err := validateELF(path, cfg.name, cfg.allowDynamicELF); err != nil {
				return err
			}
		}
	case isMachOMagic(magic):
		if cfg.platform == platformDarwin {
			info, err := normalizeMachO(path)
			if err != nil {
				return err
			}
			if !strings.Contains(path, "openssl") {
				if err := validateMachO(path, info); err != nil {
					return err
				}
			}
		}
	default:
		if err := rewriteTextFile(path); err != nil {
			return err
		}
	}
	return nil
}

func fileMagic(path string) ([8]byte, error) {
	var magic [8]byte
	file, err := os.Open(path)
	if err != nil {
		return magic, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	_, err = io.ReadFull(file, magic[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return magic, fmt.Errorf("read %s: %w", path, err)
	}
	return magic, nil
}

func stripBinary(path string) {
	_ = exec.Command("strip", "--strip-unneeded", path).Run()
}

func rewriteTextFile(path string) error {
	data, mode, err := readFile(path)
	if err != nil {
		return err
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return nil
	}

	rewritten := storeShebangPattern.ReplaceAll(data, []byte("#!/usr/bin/env $1"))
	rewritten = storeBinPattern.ReplaceAll(rewritten, nil)
	rewritten = storePathPattern.ReplaceAll(rewritten, nil)
	if bytes.Equal(data, rewritten) {
		return nil
	}
	return replaceFile(path, rewritten, mode)
}

func nukeStoreReferences(path string) error {
	data, mode, err := readFile(path)
	if err != nil {
		return err
	}
	rewritten := storeReferencePattern.ReplaceAllFunc(data, func(reference []byte) []byte {
		result := append([]byte(nil), reference...)
		copy(result[len("/nix/store/"):len("/nix/store/")+32], strings.Repeat("e", 32))
		return result
	})
	if bytes.Equal(data, rewritten) {
		return nil
	}
	return replaceFile(path, rewritten, mode)
}

func readFile(path string) ([]byte, fs.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("stat %s: %w", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", path, err)
	}
	return data, info.Mode().Perm(), nil
}

func replaceFile(path string, data []byte, mode fs.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".artifact-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := temporary.Chmod(mode | 0o200); err != nil {
		temporary.Close()
		return fmt.Errorf("chmod temporary file for %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func copyTree(source, destination string) error {
	sourceRoot, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return err
	}
	activeDirectories := make(map[string]bool)
	for _, entry := range entries {
		if err := copyNode(sourceRoot, filepath.Join(sourceRoot, entry.Name()), filepath.Join(destination, entry.Name()), activeDirectories); err != nil {
			return err
		}
	}
	return nil
}

func copyNode(sourceRoot, source, destination string, activeDirectories map[string]bool) error {
	info, err := os.Lstat(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(source)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		link, err := os.Readlink(source)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(link) && isWithin(sourceRoot, target) {
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return err
			}
			return os.Symlink(link, destination)
		}
		return copyNode(sourceRoot, target, destination, activeDirectories)
	}

	if info.IsDir() {
		resolved, err := filepath.EvalSymlinks(source)
		if err != nil {
			return err
		}
		if activeDirectories[resolved] {
			return fmt.Errorf("symlink cycle through %s", source)
		}
		activeDirectories[resolved] = true
		defer delete(activeDirectories, resolved)

		if err := os.MkdirAll(destination, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyNode(sourceRoot, filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name()), activeDirectories); err != nil {
				return err
			}
		}
		return nil
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported file type at %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm()|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func isWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func normalizeModes(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o444)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o555
		}
		return os.Chmod(path, mode)
	})
	if err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := os.Chmod(directories[index], 0o555); err != nil {
			return err
		}
	}
	return nil
}
