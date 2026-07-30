package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var archiveTime = time.Unix(0, 0).UTC()

func createArchive(root, destination, name string) (returnErr error) {
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer func() {
		returnErr = errors.Join(returnErr, output.Close())
	}()

	gzipWriter, err := gzip.NewWriterLevel(output, gzip.DefaultCompression)
	if err != nil {
		return err
	}
	gzipWriter.Header.ModTime = archiveTime
	defer func() {
		returnErr = errors.Join(returnErr, gzipWriter.Close())
	}()

	tarWriter := tar.NewWriter(gzipWriter)
	defer func() {
		returnErr = errors.Join(returnErr, tarWriter.Close())
	}()

	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		archiveName := name
		if relative != "." {
			archiveName = filepath.Join(name, relative)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(archiveName)
		if info.IsDir() {
			header.Name += "/"
		}
		header.Uid = 0
		header.Gid = 0
		header.Uname = ""
		header.Gname = ""
		if info.IsDir() || info.Mode().IsRegular() {
			header.Mode |= 0o200
		}
		header.ModTime = archiveTime
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		header.Format = tar.FormatGNU
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func cleanArchiveName(name string) (string, error) {
	cleaned := filepath.Clean(name)
	if cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid archive name %q", name)
	}
	return cleaned, nil
}
