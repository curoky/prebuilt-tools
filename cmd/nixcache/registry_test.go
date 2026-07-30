package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

func TestMissingRepositoryIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/cache/tags/list" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"errors":[{"code":"NAME_UNKNOWN","message":"repository name not known to registry"}]}`))
	}))
	defer server.Close()

	client, err := newRegistryClient(server.Listener.Addr().String()+"/cache", true)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := client.loadEntries(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries=%v", entries)
	}
}

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

	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	state := snapshot{
		ID:       "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		System:   "x86_64-linux",
		Channels: map[string]string{"nixpkgs-unstable": "revision"},
	}
	entry := testCacheEntry(t, hash, "root", []byte("compressed nar"))
	entries := map[string]cacheEntry{hash: entry}

	if err := client.pushSegment(context.Background(), state, "root", entries); err != nil {
		t.Fatal(err)
	}
	loaded, err := client.loadEntries(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded[hash]; got.NARInfo != entry.NARInfo || got.NARDigest != entry.NARDigest {
		t.Fatalf("loaded entry=%#v", got)
	}

	reader, err := client.blobReader(context.Background(), entry.NARDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
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
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	push := func(runID, narInfo string) {
		t.Helper()
		t.Setenv("GITHUB_RUN_ID", runID)
		state := snapshot{
			ID:       "sha256:" + runID + "00000000000000000000000000000000000000000000000000000000000000",
			System:   "x86_64-linux",
			Channels: map[string]string{"nixpkgs-unstable": runID},
		}
		entry := testCacheEntry(t, hash, "root", []byte("nar"))
		entry.NARInfo = strings.Replace(entry.NARInfo, "References: \n", "References: \nSystem: "+narInfo+"\n", 1)
		if err := client.pushSegment(context.Background(), state, "root", map[string]cacheEntry{hash: entry}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}

	push("1", "old")
	push("2", "new")

	entries, err := client.loadEntries(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(entries[hash].NARInfo, "System: new") {
		t.Fatalf("newest entry=%q", entries[hash].NARInfo)
	}
}

func TestFetchSegmentsSkipsDisappearedTag(t *testing.T) {
	client := testRegistryClient(t)
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	state := snapshot{
		ID:       "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		System:   "x86_64-linux",
		Channels: map[string]string{"nixpkgs-unstable": "revision"},
	}
	entry := testCacheEntry(t, hash, "root", []byte("nar"))
	if err := client.pushSegment(context.Background(), state, "root", map[string]cacheEntry{hash: entry}); err != nil {
		t.Fatal(err)
	}
	tags, err := client.listTags(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	segments, err := client.fetchSegments(context.Background(), append(tags, "v1-missing"))
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].Tag != tags[0] {
		t.Fatalf("segments=%v", segments)
	}
}

func TestFetchSegmentsCancelsRemainingRequests(t *testing.T) {
	const activeRequests = segmentLoadConcurrency - 1
	started := make(chan struct{}, segmentLoadConcurrency)
	canceled := make(chan struct{}, activeRequests)
	fail := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.Contains(request.URL.Path, "/manifests/") {
			http.NotFound(writer, request)
			return
		}
		started <- struct{}{}
		if strings.HasSuffix(request.URL.Path, "/bad") {
			<-fail
			writer.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			_, _ = writer.Write([]byte("{"))
			return
		}
		<-request.Context().Done()
		canceled <- struct{}{}
	}))
	defer server.Close()

	client, err := newRegistryClient(server.Listener.Addr().String()+"/cache", true)
	if err != nil {
		t.Fatal(err)
	}
	tags := make([]string, segmentLoadConcurrency+1)
	tags[0] = "bad"
	for index := 1; index < len(tags); index++ {
		tags[index] = fmt.Sprintf("blocked-%d", index)
	}
	result := make(chan error, 1)
	go func() {
		_, err := client.fetchSegments(context.Background(), tags)
		result <- err
	}()
	for range segmentLoadConcurrency {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("segment requests did not reach concurrency limit")
		}
	}
	close(fail)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected segment fetch error")
		}
	case <-time.After(time.Second):
		t.Fatal("segment fetch did not return after first error")
	}
	for range activeRequests {
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("active segment request was not canceled")
		}
	}
	select {
	case <-started:
		t.Fatal("queued segment request reached the registry after cancellation")
	default:
	}
}

func TestBlobReaderUsesCallerContext(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.Contains(request.URL.Path, "/blobs/") {
			http.NotFound(writer, request)
			return
		}
		close(requestStarted)
		<-request.Context().Done()
		close(requestCanceled)
	}))
	defer server.Close()

	client, err := newRegistryClient(server.Listener.Addr().String()+"/cache", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.blobReader(ctx, digest.FromBytes([]byte("nar")).String())
		result <- err
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("registry did not receive blob request")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blob request did not observe cancellation")
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("registry request context was not canceled")
	}
}

func TestValidateSegmentRejectsEntryHashMismatch(t *testing.T) {
	entry := testCacheEntry(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "root", []byte("nar"))
	item := segment{
		Version:   segmentVersion,
		Snapshot:  "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		System:    "x86_64-linux",
		CreatedAt: time.Now(),
		Entries:   map[string]cacheEntry{"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": entry},
	}
	manifest := manifestRef{
		NARLayers: map[string]ocispec.Descriptor{
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": {
				MediaType: narMediaType,
				Digest:    digest.Digest(entry.NARDigest),
				Size:      entry.NARSize,
				Annotations: map[string]string{
					storeHashAnnotation: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
		},
	}
	tag := segmentTagPrefix(item.Snapshot, item.System) + "run-random"
	if err := validateSegment(tag, item, manifest); err == nil {
		t.Fatal("expected entry hash mismatch")
	}
}

func TestGetManifestRejectsInvalidNARDescriptor(t *testing.T) {
	tests := map[string]ocispec.Descriptor{
		"media type": {
			MediaType: "application/octet-stream",
			Digest:    digest.FromBytes([]byte("nar")),
			Size:      3,
			Annotations: map[string]string{
				storeHashAnnotation: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
		"annotation": {
			MediaType: narMediaType,
			Digest:    digest.FromBytes([]byte("nar")),
			Size:      3,
		},
	}
	for name, narLayer := range tests {
		t.Run(name, func(t *testing.T) {
			client := testRegistryClient(t)
			metadata := []byte(`{"version":1}`)
			metadataDescriptor := content.NewDescriptorFromBytes(segmentMediaType, metadata)
			if err := client.repo.Blobs().Push(context.Background(), metadataDescriptor, bytes.NewReader(metadata)); err != nil {
				t.Fatal(err)
			}
			manifest, err := json.Marshal(ocispec.Manifest{
				Versioned: specs.Versioned{SchemaVersion: 2},
				MediaType: ocispec.MediaTypeImageManifest,
				Config:    metadataDescriptor,
				Layers:    []ocispec.Descriptor{metadataDescriptor, narLayer},
			})
			if err != nil {
				t.Fatal(err)
			}
			manifestDescriptor := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifest)
			if err := client.repo.PushReference(context.Background(), manifestDescriptor, bytes.NewReader(manifest), "v1-invalid"); err != nil {
				t.Fatal(err)
			}
			if _, err := client.getManifest(context.Background(), "v1-invalid"); err == nil {
				t.Fatal("expected invalid NAR descriptor error")
			}
		})
	}
}

func TestValidateSegmentRejectsNARLayerAnnotationMismatch(t *testing.T) {
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	entry := testCacheEntry(t, hash, "root", []byte("nar"))
	item := segment{
		Version:   segmentVersion,
		Snapshot:  "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		System:    "x86_64-linux",
		CreatedAt: time.Now(),
		Entries:   map[string]cacheEntry{hash: entry},
	}
	manifest := manifestRef{
		NARLayers: map[string]ocispec.Descriptor{
			hash: {
				MediaType: narMediaType,
				Digest:    digest.Digest(entry.NARDigest),
				Size:      entry.NARSize,
				Annotations: map[string]string{
					storeHashAnnotation: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				},
			},
		},
	}
	tag := segmentTagPrefix(item.Snapshot, item.System) + "run-random"
	if err := validateSegment(tag, item, manifest); err == nil {
		t.Fatal("expected NAR layer annotation mismatch")
	}
}
