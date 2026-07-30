package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

type registryClient struct {
	repo *remote.Repository
}

func newRegistryClient(repository string, insecure bool) (*registryClient, error) {
	repo, err := remote.NewRepository(repository)
	if err != nil {
		return nil, fmt.Errorf("parse cache repository: %w", err)
	}
	repo.PlainHTTP = insecure

	var credential auth.CredentialFunc
	authSource := "anonymous"
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		username := os.Getenv("GITHUB_ACTOR")
		if username == "" {
			username = "token"
		}
		credential = auth.StaticCredential(repo.Reference.Registry, auth.Credential{
			Username: username,
			Password: token,
		})
		authSource = fmt.Sprintf("GITHUB_TOKEN (user %q)", username)
	} else {
		store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
		if err != nil {
			return nil, fmt.Errorf("load registry credentials: %w", err)
		}
		credential = credentials.Credential(store)
		authSource = "docker credential store"
	}
	client := *auth.DefaultClient
	client.Credential = credential
	repo.Client = &client
	// Startup overview to confirm which repository is targeted and how it
	// authenticates. Never logs the token value, only its source and username,
	// so it is safe under `set -x`-style CI logs.
	log.Printf("cache client: repo=%s registry=%s plain-http=%t auth=%s",
		repo.Reference.Repository, repo.Reference.Registry, repo.PlainHTTP, authSource)
	return &registryClient{repo: repo}, nil
}

// segmentRef pairs a decoded segment with the registry metadata needed to
// prune it: its tag, the manifest descriptor to delete, and the on-registry
// byte size of every layer this manifest references.
type segmentRef struct {
	segment
	Tag        string
	Manifest   ocispec.Descriptor
	LayerSizes map[digest.Digest]int64
}

// registryTimeout bounds each individual registry round-trip (tag listing,
// manifest fetch, blob fetch). Without it the initial index load can block
// indefinitely on a slow or wedged GHCR request, so `serveCache` never marks
// the index ready and CI probes exhaust their retries on 503.
const registryTimeout = 30 * time.Second

func (client *registryClient) listSegments(tagPrefix string) ([]segmentRef, error) {
	log.Printf("listing cache tags from %s", client.repo.Reference.Repository)
	ctx, cancel := context.WithTimeout(context.Background(), registryTimeout)
	tags, err := registry.Tags(ctx, client.repo)
	cancel()
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) || isNameUnknown(err) {
			log.Printf("cache repository %s not found yet, treating as empty", client.repo.Reference.Repository)
			return nil, nil
		}
		return nil, fmt.Errorf("list cache segments: %w", err)
	}
	log.Printf("cache tags fetched: %d total", len(tags))

	// When a tagPrefix is given (serve mode), only tags for the current
	// snapshot+system can ever be cache hits, so skip fetching every other
	// segment's manifest. This is the difference between a handful of round
	// trips and hundreds. prune/size pass an empty prefix and load everything.
	match := segmentPrefix
	if tagPrefix != "" {
		match = tagPrefix
	}
	segmentTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		if strings.HasPrefix(tag, match) {
			segmentTags = append(segmentTags, tag)
		}
	}
	log.Printf("segment tags to load: %d (prefix %q)", len(segmentTags), match)

	segments, err := client.fetchSegments(segmentTags)
	if err != nil {
		return nil, err
	}
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].CreatedAt.Before(segments[j].CreatedAt)
	})
	log.Printf("listed cache: %d tag(s), %d segment(s)", len(tags), len(segments))
	return segments, nil
}

// segmentLoadConcurrency bounds how many segment manifests are fetched in
// parallel. Loading is O(number of tags for the snapshot) network round trips;
// a snapshot that never changes accumulates one tag per CI run, so serve can
// face hundreds. Fetching serially takes minutes, so fan out with a fixed
// worker pool.
const segmentLoadConcurrency = 32

// fetchSegments loads every tag's segment concurrently, capped at
// segmentLoadConcurrency. The first error cancels the batch. Order is not
// preserved; the caller sorts.
func (client *registryClient) fetchSegments(tags []string) ([]segmentRef, error) {
	type result struct {
		ref segmentRef
		err error
	}
	results := make(chan result, len(tags))
	sem := make(chan struct{}, segmentLoadConcurrency)
	var wg sync.WaitGroup
	for _, tag := range tags {
		wg.Add(1)
		go func(tag string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ref, err := client.getSegment(tag)
			if err != nil {
				results <- result{err: fmt.Errorf("load segment %s: %w", tag, err)}
				return
			}
			results <- result{ref: ref}
		}(tag)
	}
	go func() { wg.Wait(); close(results) }()

	segments := make([]segmentRef, 0, len(tags))
	var loadErr error
	var done int
	for res := range results {
		if res.err != nil {
			if loadErr == nil {
				loadErr = res.err
			}
			continue
		}
		segments = append(segments, res.ref)
		done++
		if done%100 == 0 || done == len(tags) {
			log.Printf("loaded %d/%d segments", done, len(tags))
		}
	}
	if loadErr != nil {
		return nil, loadErr
	}
	return segments, nil
}

// deleteSegment removes a segment manifest by tag. GHCR drops the tag with the
// manifest; layers no longer referenced by any manifest become untagged and are
// reclaimed by the registry's own garbage collection.
func (client *registryClient) deleteSegment(ref segmentRef) error {
	return client.repo.Delete(context.Background(), ref.Manifest)
}

func isNameUnknown(err error) bool {
	var response *errcode.ErrorResponse
	if !errors.As(err, &response) {
		return false
	}
	for _, item := range response.Errors {
		if item.Code == errcode.ErrorCodeNameUnknown {
			return true
		}
	}
	return false
}

func (client *registryClient) getSegment(tag string) (segmentRef, error) {
	ctx, cancel := context.WithTimeout(context.Background(), registryTimeout)
	defer cancel()
	manifestDescriptor, reader, err := client.repo.FetchReference(ctx, tag)
	if err != nil {
		return segmentRef{}, err
	}
	defer reader.Close()

	var manifest ocispec.Manifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return segmentRef{}, err
	}
	if len(manifest.Layers) == 0 {
		return segmentRef{}, fmt.Errorf("segment has no metadata layer")
	}
	metadata := manifest.Layers[0]
	if metadata.MediaType != segmentMediaType {
		return segmentRef{}, fmt.Errorf("unexpected metadata media type %q", metadata.MediaType)
	}
	reader, err = client.repo.Fetch(ctx, metadata)
	if err != nil {
		return segmentRef{}, err
	}
	defer reader.Close()

	var item segment
	if err := json.NewDecoder(reader).Decode(&item); err != nil {
		return segmentRef{}, fmt.Errorf("decode segment metadata: %w", err)
	}
	if item.Version != segmentVersion {
		return segmentRef{}, fmt.Errorf("unsupported segment version %d", item.Version)
	}

	layerSizes := make(map[digest.Digest]int64, len(manifest.Layers))
	for _, layer := range manifest.Layers {
		layerSizes[layer.Digest] = layer.Size
	}
	return segmentRef{
		segment:    item,
		Tag:        tag,
		Manifest:   manifestDescriptor,
		LayerSizes: layerSizes,
	}, nil
}

func (client *registryClient) loadEntries(tagPrefix string) (map[string]cacheEntry, error) {
	segments, err := client.listSegments(tagPrefix)
	if err != nil {
		return nil, err
	}
	entries := make(map[string]cacheEntry)
	snapshots := make(map[string]struct{})
	for _, item := range segments {
		snapshots[item.Snapshot] = struct{}{}
		for hash, entry := range item.Entries {
			entries[hash] = entry
		}
	}
	log.Printf("loaded %d segment(s) across %d snapshot(s): %d entries", len(segments), len(snapshots), len(entries))
	return entries, nil
}

func (client *registryClient) blobReader(digest string) (io.ReadCloser, error) {
	_, reader, err := client.repo.Blobs().FetchReference(context.Background(), digest)
	return reader, err
}

func (client *registryClient) pushSegment(state snapshot, entries map[string]cacheEntry) error {
	item := segment{
		Version:          segmentVersion,
		Snapshot:         state.ID,
		RepositoryCommit: state.RepositoryCommit,
		System:           state.System,
		CreatedAt:        time.Now().UTC(),
		Channels:         state.Channels,
		Entries:          entries,
	}
	metadata, err := json.Marshal(item)
	if err != nil {
		return err
	}
	ctx := context.Background()
	metadataDescriptor := content.NewDescriptorFromBytes(segmentMediaType, metadata)
	err = client.repo.Blobs().Push(ctx, metadataDescriptor, bytes.NewReader(metadata))
	if err != nil {
		return err
	}

	hashes := slices.Sorted(maps.Keys(entries))
	layers := make([]ocispec.Descriptor, 1, len(entries)+1)
	layers[0] = metadataDescriptor
	var uploaded, skipped int
	for _, hash := range hashes {
		entry := entries[hash]
		descriptor := ocispec.Descriptor{
			MediaType: narMediaType,
			Digest:    digest.Digest(entry.NARDigest),
			Size:      entry.NARSize,
		}
		descriptor.Annotations = map[string]string{"org.nixos.store.hash": hash}
		pushed, err := client.pushFile(ctx, descriptor, entry.NARPath)
		if err != nil {
			return err
		}
		if pushed {
			uploaded++
		} else {
			skipped++
		}
		layers = append(layers, descriptor)
	}
	log.Printf("pushed %d NAR blob(s), skipped %d already present", uploaded, skipped)

	manifest, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    metadataDescriptor,
		Layers:    layers,
	})
	if err != nil {
		return err
	}
	manifestDescriptor := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifest)
	tag := client.segmentTag(state)
	if err := client.repo.PushReference(ctx, manifestDescriptor, bytes.NewReader(manifest), tag); err != nil {
		return fmt.Errorf("publish segment %s: %w", tag, err)
	}
	log.Printf("published segment %s (%d entries)", tag, len(entries))
	return nil
}

func (client *registryClient) pushFile(ctx context.Context, descriptor ocispec.Descriptor, path string) (bool, error) {
	exists, err := client.repo.Blobs().Exists(ctx, descriptor)
	if err != nil || exists {
		return false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	if err := client.repo.Blobs().Push(ctx, descriptor, file); err != nil {
		return false, err
	}
	return true, nil
}

func (client *registryClient) segmentTag(state snapshot) string {
	runID := os.Getenv("GITHUB_RUN_ID")
	if runID == "" {
		runID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%s%s-%s", segmentTagPrefix(state.ID, state.System), runID, rand.Text())
}

// segmentTagPrefix is the tag namespace shared by every segment of a given
// snapshot+system: `v1-<snapshot16>-<system>-`. serve filters tags by this
// prefix so it only fetches manifests that can actually be cache hits, and
// segmentTag appends `<runID>-<rand>` to make each run's tag unique within it.
func segmentTagPrefix(snapshotID, system string) string {
	short := strings.TrimPrefix(snapshotID, "sha256:")
	if len(short) > 16 {
		short = short[:16]
	}
	return fmt.Sprintf("%s%s-%s-", segmentPrefix, short, system)
}
