package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2/content"
)

func TestServeCache(t *testing.T) {
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
	entry := cacheEntry{
		StorePath: "/nix/store/" + hash + "-root",
		NARURL:    "nar/root.nar.zst",
		NARDigest: blob.Digest.String(),
		NARSize:   blob.Size,
		NARInfo:   narInfo,
		NARPath:   narPath,
	}
	if err := client.pushSegment(state, map[string]cacheEntry{hash: entry}); err != nil {
		t.Fatal(err)
	}
	index := newCacheIndex(client, "")
	if _, err := index.refresh(); err != nil {
		t.Fatal(err)
	}
	index.ready.Store(true)
	server := httptest.NewServer(http.HandlerFunc(index.serveHTTP))
	t.Cleanup(server.Close)

	assertResponse := func(method, path, contentType, body string) {
		t.Helper()
		request, err := http.NewRequest(method, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s %s status=%d", method, path, response.StatusCode)
		}
		if got := response.Header.Get("Content-Type"); got != contentType {
			t.Fatalf("%s content type=%q want %q", path, got, contentType)
		}
		got, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != body {
			t.Fatalf("%s body=%q want %q", path, got, body)
		}
	}

	assertResponse(http.MethodGet, "/nix-cache-info", "text/x-nix-cache-info", nixCacheInfo)
	assertResponse(http.MethodGet, "/"+hash+".narinfo", "text/x-nix-narinfo", narInfo)
	assertResponse(http.MethodGet, "/nar/root.nar.zst", "application/x-nix-nar", "compressed nar")
	assertResponse(http.MethodHead, "/nar/root.nar.zst", "application/x-nix-nar", "")

	response, err := http.Get(server.URL + "/missing.narinfo")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status=%d", response.StatusCode)
	}
}

func TestServeCacheReturnsBadGatewayBeforeWritingNARHeaders(t *testing.T) {
	client := testRegistryClient(t)
	index := newCacheIndex(client, "")
	index.nars["nar/missing.nar.zst"] = cacheEntry{
		NARURL:    "nar/missing.nar.zst",
		NARDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		NARSize:   42,
	}

	request := httptest.NewRequest(http.MethodGet, "/nar/missing.nar.zst", nil)
	response := httptest.NewRecorder()
	index.serveHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status=%d", response.Code)
	}
}
