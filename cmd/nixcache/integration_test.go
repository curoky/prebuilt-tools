package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNixRoundTrip(t *testing.T) {
	storePath := os.Getenv("NIXCACHE_TEST_STORE_PATH")
	if storePath == "" {
		t.Skip("set NIXCACHE_TEST_STORE_PATH to run the Nix integration test")
	}
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix is not available")
	}

	client := testRegistryClient(t)
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := pushPaths(context.Background(), client, repoRoot, "integration-test", []string{storePath}); err != nil {
		t.Fatal(err)
	}

	index := newCacheIndex(client, "")
	if _, err := index.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	index.ready.Store(true)
	server := httptest.NewServer(http.HandlerFunc(index.serveHTTP))
	t.Cleanup(server.Close)

	destination := "file://" + filepath.Join(t.TempDir(), "cache")
	command := exec.Command(
		nix,
		"copy",
		"--no-check-sigs",
		"--from", server.URL,
		"--to", destination,
		storePath,
	)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}

	hash, err := storeHash(storePath)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := strings.TrimPrefix(destination, "file://")
	if _, err := os.Stat(filepath.Join(cacheDir, hash+".narinfo")); err != nil {
		t.Fatalf("copied narinfo is missing: %v", err)
	}
}
