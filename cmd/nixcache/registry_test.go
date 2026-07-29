package main

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"oras.land/oras-go/v2/content"
)

func testRegistryClient(t *testing.T) *registryClient {
	t.Helper()
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)

	client, err := newRegistryClient(server.Listener.Addr().String()+"/cache", true)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestRegistryRoundTrip(t *testing.T) {
	client := testRegistryClient(t)
	narPath := filepath.Join(t.TempDir(), "root.nar.zst")
	if err := os.WriteFile(narPath, []byte("compressed nar"), 0o644); err != nil {
		t.Fatal(err)
	}
	blob := content.NewDescriptorFromBytes(narMediaType, []byte("compressed nar"))

	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	narInfo := "StorePath: /nix/store/" + hash + "-root\nURL: nar/root.nar.zst\nCompression: zstd\n"
	state := snapshot{
		ID:       "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		System:   "x86_64-linux",
		Channels: map[string]string{"nixpkgs-unstable": "revision"},
	}
	entries := map[string]cacheEntry{
		hash: {
			StorePath: "/nix/store/" + hash + "-root",
			NARURL:    "nar/root.nar.zst",
			NARDigest: blob.Digest.String(),
			NARSize:   blob.Size,
			NARInfo:   narInfo,
			NARPath:   narPath,
		},
	}

	if err := client.pushSegment(state, entries); err != nil {
		t.Fatal(err)
	}
	loaded, err := client.loadEntries()
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded[hash]; got.NARInfo != narInfo || got.NARDigest != blob.Digest.String() {
		t.Fatalf("loaded entry=%#v", got)
	}

	reader, err := client.blobReader(blob.Digest.String())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "compressed nar" {
		t.Fatalf("blob=%q", body)
	}
}

func TestNewestSegmentWins(t *testing.T) {
	client := testRegistryClient(t)
	path := filepath.Join(t.TempDir(), "nar")
	if err := os.WriteFile(path, []byte("nar"), 0o644); err != nil {
		t.Fatal(err)
	}
	blob := content.NewDescriptorFromBytes(narMediaType, []byte("nar"))
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	push := func(runID, narInfo string) {
		t.Helper()
		t.Setenv("GITHUB_RUN_ID", runID)
		state := snapshot{
			ID:       "sha256:" + runID + "00000000000000000000000000000000000000000000000000000000000000",
			System:   "x86_64-linux",
			Channels: map[string]string{"nixpkgs-unstable": runID},
		}
		entry := cacheEntry{
			StorePath: "/nix/store/" + hash + "-root",
			NARURL:    "nar/root.nar.zst",
			NARDigest: blob.Digest.String(),
			NARSize:   blob.Size,
			NARInfo:   narInfo,
			NARPath:   path,
		}
		if err := client.pushSegment(state, map[string]cacheEntry{hash: entry}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}

	push("1", "old")
	push("2", "new")

	entries, err := client.loadEntries()
	if err != nil {
		t.Fatal(err)
	}
	if entries[hash].NARInfo != "new" {
		t.Fatalf("newest entry=%q", entries[hash].NARInfo)
	}
}
