package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	var metadata meta
	data, err := os.ReadFile(metaPath(prefix, name))
	if err != nil {
		return metadata, err
	}
	return metadata, json.Unmarshal(data, &metadata)
}

func writeMeta(prefix string, metadata meta) error {
	return writeMetaAt(storePath(prefix, metadata.Name), metadata)
}

func writeMetaAt(store string, metadata meta) error {
	metadata.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(store, metaFile), 0o644, bytes.NewReader(data))
}

func writeAtomic(path string, mode os.FileMode, reader io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := io.Copy(file, reader); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// extractTarGz strips the archive's package-name prefix and rejects entries
// that could escape dst through paths, symlinks, or hardlinks.
func extractTarGz(src, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		rel := stripFirstComponent(header.Name)
		if rel == "" {
			continue
		}
		target, err := safeArchivePath(dst, rel)
		if err != nil {
			return fmt.Errorf("unsafe path in archive: %s", header.Name)
		}
		if err := rejectSymlinkParents(dst, target); err != nil {
			return fmt.Errorf("unsafe path in archive %s: %w", header.Name, err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0o755)
		case tar.TypeReg, tar.TypeRegA:
			if info, statErr := os.Lstat(target); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("unsafe symlink target in archive: %s", header.Name)
			} else if statErr != nil && !os.IsNotExist(statErr) {
				return statErr
			}
			err = writeFile(target, tarReader, os.FileMode(header.Mode)&0o777)
		case tar.TypeSymlink:
			linkTarget, linkErr := safeSymlinkTarget(dst, target, header.Linkname)
			if linkErr != nil {
				return fmt.Errorf("unsafe symlink in archive %s: %w", header.Name, linkErr)
			}
			if err = os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
				if removeErr := os.Remove(target); removeErr != nil && !os.IsNotExist(removeErr) {
					return removeErr
				}
				relativeTarget, relErr := filepath.Rel(filepath.Dir(target), linkTarget)
				if relErr != nil {
					return relErr
				}
				err = os.Symlink(relativeTarget, target)
			}
		case tar.TypeLink:
			linkRel := stripFirstComponent(header.Linkname)
			if linkRel == "" {
				return fmt.Errorf("invalid hardlink target in archive: %s", header.Linkname)
			}
			linkTarget, linkErr := safeArchivePath(dst, linkRel)
			if linkErr != nil {
				return fmt.Errorf("unsafe hardlink in archive %s: %w", header.Name, linkErr)
			}
			if err = rejectSymlinkParents(dst, linkTarget); err != nil {
				return fmt.Errorf("unsafe hardlink in archive %s: %w", header.Name, err)
			}
			if err = os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
				err = os.Link(linkTarget, target)
			}
		default:
			return fmt.Errorf("unsupported tar entry type %d for %s", header.Typeflag, header.Name)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func safeArchivePath(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute path")
	}
	target := filepath.Clean(filepath.Join(root, rel))
	if !withinRoot(root, target) {
		return "", fmt.Errorf("path escapes extraction root")
	}
	return target, nil
}

func safeSymlinkTarget(root, linkPath, linkname string) (string, error) {
	if filepath.IsAbs(linkname) {
		return "", fmt.Errorf("absolute target")
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(linkPath), filepath.FromSlash(linkname)))
	if !withinRoot(root, target) {
		return "", fmt.Errorf("target escapes extraction root")
	}
	return target, nil
}

func withinRoot(root, path string) bool {
	root = filepath.Clean(root)
	return path != root && strings.HasPrefix(path, root+string(os.PathSeparator))
}

func rejectSymlinkParents(root, target string) error {
	rel, err := filepath.Rel(root, filepath.Dir(target))
	if err != nil {
		return err
	}
	current := filepath.Clean(root)
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		switch {
		case os.IsNotExist(err):
			return nil
		case err != nil:
			return err
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("parent %s is a symlink", current)
		case !info.IsDir():
			return fmt.Errorf("parent %s is not a directory", current)
		}
	}
	return nil
}

func writeFile(target string, reader io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, reader)
	return err
}

func stripFirstComponent(name string) string {
	name = strings.TrimPrefix(filepath.ToSlash(name), "./")
	parts := strings.SplitN(name, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func walkPkgFiles(store string, fn func(abs, rel string) error) error {
	return filepath.Walk(store, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(store, path)
		if err != nil {
			return err
		}
		if rel == metaFile {
			return nil
		}
		return fn(path, rel)
	})
}

func linkPkg(prefix, name string) error {
	return linkPkgInto(prefix, name, prefix)
}

// linkPkgInto performs a complete conflict preflight before creating links, so
// a collision cannot leave a partially linked package.
func linkPkgInto(prefix, name, root string) error {
	store := storePath(prefix, name)
	if err := walkPkgFiles(store, func(abs, rel string) error {
		dest := filepath.Join(root, rel)
		if err := rejectSymlinkParents(root, dest); err != nil {
			return fmt.Errorf("%s has unsafe parent: %w", dest, err)
		}
		info, err := os.Lstat(dest)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s conflicts with package %s", dest, name)
		}
		owned, err := symlinkPointsTo(dest, abs)
		if err != nil {
			return err
		}
		if !owned {
			return fmt.Errorf("%s is already linked by another package", dest)
		}
		return nil
	}); err != nil {
		return err
	}

	return walkPkgFiles(store, func(abs, rel string) error {
		dest := filepath.Join(root, rel)
		if err := rejectSymlinkParents(root, dest); err != nil {
			return fmt.Errorf("%s has unsafe parent: %w", dest, err)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		relativeTarget, err := filepath.Rel(filepath.Dir(dest), abs)
		if err != nil {
			return err
		}
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Symlink(relativeTarget, dest)
	})
}

func unlinkPkg(prefix, name string) error {
	return unlinkPkgFrom(prefix, name, prefix)
}

func unlinkPkgFrom(prefix, name, root string) error {
	return walkPkgFiles(storePath(prefix, name), func(abs, rel string) error {
		dest := filepath.Join(root, rel)
		if err := rejectSymlinkParents(root, dest); err != nil {
			return fmt.Errorf("%s has unsafe parent: %w", dest, err)
		}
		owned, err := symlinkPointsTo(dest, abs)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if owned {
			return os.Remove(dest)
		}
		return nil
	})
}

func symlinkPointsTo(link, target string) (bool, error) {
	info, err := os.Lstat(link)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	linkTarget, err := os.Readlink(link)
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(linkTarget) {
		linkTarget = filepath.Join(filepath.Dir(link), linkTarget)
	}
	return filepath.Clean(linkTarget) == filepath.Clean(target), nil
}

func backupPath(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	backup, err := os.MkdirTemp(filepath.Dir(path), "."+filepath.Base(path)+".backup-")
	if err != nil {
		return "", err
	}
	if err := os.Remove(backup); err != nil {
		return "", err
	}
	return backup, nil
}

// replaceStore swaps a staged package into place and restores the old store and
// links if linking the new package fails.
func replaceStore(prefix, name, staged string, wasLinked, linked bool) error {
	store := storePath(prefix, name)
	backup, err := backupPath(store)
	if err != nil {
		return err
	}

	if wasLinked {
		if err := unlinkPkg(prefix, name); err != nil {
			return fmt.Errorf("%s: unlink failed: %w", name, err)
		}
	}
	if backup != "" {
		if err := os.Rename(store, backup); err != nil {
			if wasLinked {
				_ = linkPkg(prefix, name)
			}
			return err
		}
	}
	if err := os.Rename(staged, store); err != nil {
		if backup != "" {
			_ = os.Rename(backup, store)
		}
		if wasLinked {
			_ = linkPkg(prefix, name)
		}
		return err
	}
	if linked {
		if err := linkPkg(prefix, name); err != nil {
			_ = unlinkPkg(prefix, name)
			_ = os.RemoveAll(store)
			if backup != "" {
				_ = os.Rename(backup, store)
			}
			if wasLinked {
				_ = linkPkg(prefix, name)
			}
			return fmt.Errorf("%s: link failed: %w", name, err)
		}
	}
	if backup != "" {
		return os.RemoveAll(backup)
	}
	return nil
}
