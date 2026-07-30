package main

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/nix-community/go-nix/pkg/nixhash"
	"github.com/opencontainers/go-digest"
)

func testCacheEntry(t *testing.T, hash, name string, body []byte) cacheEntry {
	t.Helper()
	sum := sha256.Sum256(body)
	fileHash := nixhash.MustNewHash(nixhash.SHA256, sum[:]).Format(nixhash.NixBase32, true)
	narURL := "nar/" + digest.FromBytes(body).Encoded() + ".nar.zst"
	narPath := filepath.Join(t.TempDir(), filepath.FromSlash(narURL))
	if err := os.MkdirAll(filepath.Dir(narPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(narPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	storePath := "/nix/store/" + hash + "-" + name
	narInfo := "StorePath: " + storePath + "\n" +
		"URL: " + narURL + "\n" +
		"Compression: zstd\n" +
		"FileHash: " + fileHash + "\n" +
		"FileSize: " + strconv.FormatInt(int64(len(body)), 10) + "\n" +
		"NarHash: " + fileHash + "\n" +
		"NarSize: " + strconv.FormatInt(int64(len(body)), 10) + "\n" +
		"References: \n"
	return cacheEntry{
		StorePath: storePath,
		NARURL:    narURL,
		NARDigest: digest.FromBytes(body).String(),
		NARSize:   int64(len(body)),
		NARInfo:   narInfo,
		NARPath:   narPath,
	}
}
