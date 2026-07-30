package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/nix-community/go-nix/pkg/narinfo"
	"github.com/opencontainers/go-digest"
)

func pushPaths(ctx context.Context, client *registryClient, repoRoot, packageKey string, storePaths []string) error {
	state, err := loadSnapshot(repoRoot)
	if err != nil {
		return err
	}
	log.Printf("pushing %d store path(s) for %s package %s snapshot %s", len(storePaths), state.System, packageKey, state.ID)

	cacheDir, err := os.MkdirTemp("", "nixcache-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(cacheDir) }()

	query := url.Values{
		"compression":          {"zstd"},
		"parallel-compression": {"true"},
	}
	if key := os.Getenv("NIX_SIGNING_KEY_FILE"); key != "" {
		query.Set("secret-key", key)
	}
	destination := url.URL{
		Scheme:   "file",
		Path:     cacheDir,
		RawQuery: query.Encode(),
	}
	args := append([]string{"copy", "--to", destination.String()}, storePaths...)
	command := exec.CommandContext(ctx, "nix", args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("nix copy: %w", err)
	}
	entries, err := readCache(cacheDir)
	if err != nil {
		return err
	}
	log.Printf("collected %d cache entr(ies) from nix copy", len(entries))
	return client.pushSegment(ctx, state, packageKey, entries)
}

func readCache(cacheDir string) (map[string]cacheEntry, error) {
	files, err := filepath.Glob(filepath.Join(cacheDir, "*"+narInfoSuffix))
	if err != nil {
		return nil, err
	}
	entries := make(map[string]cacheEntry, len(files))
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		info, err := narinfo.Parse(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.Base(file), err)
		}
		if err := info.Check(); err != nil {
			return nil, fmt.Errorf("check %s: %w", filepath.Base(file), err)
		}
		if path.Clean(info.URL) != info.URL || !strings.HasPrefix(info.URL, "nar/") {
			return nil, fmt.Errorf("invalid NAR URL %q", info.URL)
		}
		narPath := filepath.Join(cacheDir, filepath.FromSlash(info.URL))
		if info.FileHash == nil || info.FileHash.Algo().String() != digest.SHA256.String() || info.FileSize == 0 {
			return nil, fmt.Errorf("invalid NAR file metadata for %q", info.URL)
		}
		hash, err := storeHash(info.StorePath)
		if err != nil {
			return nil, err
		}
		entries[hash] = cacheEntry{
			StorePath: info.StorePath,
			NARURL:    info.URL,
			NARDigest: digest.NewDigestFromEncoded(
				digest.SHA256,
				digest.SHA256.Encode(info.FileHash.Digest()),
			).String(),
			NARSize: int64(info.FileSize),
			NARInfo: string(body),
			NARPath: narPath,
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("nix copy produced no narinfo files")
	}
	return entries, nil
}
