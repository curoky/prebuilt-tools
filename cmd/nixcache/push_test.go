package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
)

func TestReadCache(t *testing.T) {
	cacheDir := t.TempDir()
	narURL := "nar/root.nar.zst"
	narBody := []byte("compressed nar")
	if err := os.Mkdir(filepath.Join(cacheDir, "nar"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, filepath.FromSlash(narURL)), narBody, 0o644); err != nil {
		t.Fatal(err)
	}

	hash := "c2h2f4cw9p8i8zcfy52fd1dd6g0yhnki"
	narInfo := "StorePath: /nix/store/" + hash + "-root\n" +
		"URL: " + narURL + "\n" +
		"Compression: zstd\n" +
		"FileHash: sha256:0qczlpkw79b8gfzh97i7gkb0c5mp98mcb78gj0braxm1x9y5qw0m\n" +
		"FileSize: 14\n" +
		"NarHash: sha256:0jvbywkmjaq0rxzvw9yi1rcpv4y57j23m7xhhhjd3isq93qldr6i\n" +
		"NarSize: 14\n" +
		"References: \n"
	if err := os.WriteFile(filepath.Join(cacheDir, hash+narInfoSuffix), []byte(narInfo), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := readCache(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	entry := entries[hash]
	if entry.NARInfo != narInfo || entry.NARDigest != digest.FromBytes(narBody).String() ||
		entry.NARSize != int64(len(narBody)) ||
		entry.NARPath != filepath.Join(cacheDir, filepath.FromSlash(narURL)) {
		t.Fatalf("entry=%#v", entry)
	}
}
