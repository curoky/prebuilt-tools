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
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nix-community/go-nix/pkg/narinfo"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
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
	var authSource string
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

type segmentRef struct {
	segment
	Tag string
}

type manifestRef struct {
	Tag       string
	Metadata  ocispec.Descriptor
	NARLayers map[string]ocispec.Descriptor
}

const storeHashAnnotation = "org.nixos.store.hash"

// registryTimeout bounds each individual registry round-trip (tag listing,
// manifest fetch, blob fetch). Without it the initial index load can block
// indefinitely on a slow or wedged GHCR request, so `serveCache` never marks
// the index ready and CI probes exhaust their retries on 503.
const registryTimeout = 30 * time.Second

func (client *registryClient) listTags(ctx context.Context, tagPrefix string) ([]string, error) {
	log.Printf("listing cache tags from %s", client.repo.Reference.Repository)
	requestCtx, cancel := context.WithTimeout(ctx, registryTimeout)
	defer cancel()
	tags, err := registry.Tags(requestCtx, client.repo)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) || isNameUnknown(err) {
			log.Printf("cache repository %s not found yet, treating as empty", client.repo.Reference.Repository)
			return nil, nil
		}
		return nil, fmt.Errorf("list cache segments: %w", err)
	}
	log.Printf("cache tags fetched: %d total", len(tags))

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
	return segmentTags, nil
}

func (client *registryClient) listSegments(ctx context.Context, tagPrefix string) ([]segmentRef, error) {
	tags, err := client.listTags(ctx, tagPrefix)
	if err != nil {
		return nil, err
	}
	segments, err := client.fetchSegments(ctx, tags)
	if err != nil {
		return nil, err
	}
	sort.Slice(segments, func(i, j int) bool {
		if !segments[i].CreatedAt.Equal(segments[j].CreatedAt) {
			return segments[i].CreatedAt.Before(segments[j].CreatedAt)
		}
		return segments[i].Tag < segments[j].Tag
	})
	log.Printf("listed cache: %d segment(s)", len(segments))
	return segments, nil
}

// segmentLoadConcurrency bounds how many segment manifests are fetched in
// parallel. Loading is O(number of tags for the snapshot) network round trips;
// a snapshot that never changes accumulates one tag per CI run, so serve can
// face hundreds. Fetching serially takes minutes, so fan out with a fixed
// worker pool.
const segmentLoadConcurrency = 32

func (client *registryClient) listManifests(ctx context.Context, tagPrefix string) ([]manifestRef, error) {
	tags, err := client.listTags(ctx, tagPrefix)
	if err != nil {
		return nil, err
	}
	manifests := make([]manifestRef, len(tags))
	missing := make([]bool, len(tags))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(segmentLoadConcurrency)
	for index, tag := range tags {
		group.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return err
			}
			ref, err := client.getManifest(groupCtx, tag)
			if err != nil {
				if errors.Is(err, errdef.ErrNotFound) {
					missing[index] = true
					log.Printf("segment %s disappeared while loading, skipping", tag)
					return nil
				}
				return fmt.Errorf("load manifest %s: %w", tag, err)
			}
			manifests[index] = ref
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	loaded := manifests[:0]
	for index, ref := range manifests {
		if !missing[index] {
			loaded = append(loaded, ref)
		}
	}
	return loaded, nil
}

func (client *registryClient) fetchSegments(ctx context.Context, tags []string) ([]segmentRef, error) {
	segments := make([]segmentRef, len(tags))
	missing := make([]bool, len(tags))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(segmentLoadConcurrency)
	for index, tag := range tags {
		group.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return err
			}
			ref, err := client.getSegment(groupCtx, tag)
			if err != nil {
				if errors.Is(err, errdef.ErrNotFound) {
					missing[index] = true
					log.Printf("segment %s disappeared while loading, skipping", tag)
					return nil
				}
				return fmt.Errorf("load segment %s: %w", tag, err)
			}
			segments[index] = ref
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	loaded := segments[:0]
	for index, ref := range segments {
		if !missing[index] {
			loaded = append(loaded, ref)
		}
	}
	log.Printf("loaded %d/%d segments", len(loaded), len(tags))
	return loaded, nil
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

func (client *registryClient) getManifest(ctx context.Context, tag string) (manifestRef, error) {
	requestCtx, cancel := context.WithTimeout(ctx, registryTimeout)
	defer cancel()
	_, reader, err := client.repo.FetchReference(requestCtx, tag)
	if err != nil {
		return manifestRef{}, err
	}
	defer func() { _ = reader.Close() }()

	var manifest ocispec.Manifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return manifestRef{}, err
	}
	if len(manifest.Layers) == 0 {
		return manifestRef{}, fmt.Errorf("segment has no metadata layer")
	}
	metadata := manifest.Layers[0]
	if metadata.MediaType != segmentMediaType {
		return manifestRef{}, fmt.Errorf("unexpected metadata media type %q", metadata.MediaType)
	}

	if metadata.Size < 0 {
		return manifestRef{}, fmt.Errorf("metadata layer has invalid size %d", metadata.Size)
	}
	narLayers := make(map[string]ocispec.Descriptor, len(manifest.Layers)-1)
	for _, layer := range manifest.Layers[1:] {
		if layer.MediaType != narMediaType {
			return manifestRef{}, fmt.Errorf("unexpected NAR media type %q", layer.MediaType)
		}
		if layer.Size < 0 {
			return manifestRef{}, fmt.Errorf("layer %s has invalid size %d", layer.Digest, layer.Size)
		}
		hash := layer.Annotations[storeHashAnnotation]
		if hash == "" {
			return manifestRef{}, fmt.Errorf("NAR layer %s is missing %s annotation", layer.Digest, storeHashAnnotation)
		}
		if _, exists := narLayers[hash]; exists {
			return manifestRef{}, fmt.Errorf("duplicate NAR layer for store hash %s", hash)
		}
		narLayers[hash] = layer
	}
	return manifestRef{Tag: tag, Metadata: metadata, NARLayers: narLayers}, nil
}

func (client *registryClient) getSegment(ctx context.Context, tag string) (segmentRef, error) {
	manifest, err := client.getManifest(ctx, tag)
	if err != nil {
		return segmentRef{}, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, registryTimeout)
	defer cancel()
	reader, err := client.repo.Fetch(requestCtx, manifest.Metadata)
	if err != nil {
		return segmentRef{}, err
	}
	defer func() { _ = reader.Close() }()

	var item segment
	if err := json.NewDecoder(reader).Decode(&item); err != nil {
		return segmentRef{}, fmt.Errorf("decode segment metadata: %w", err)
	}
	if item.Version != segmentVersion {
		return segmentRef{}, fmt.Errorf("unsupported segment version %d", item.Version)
	}
	if err := validateSegment(tag, item, manifest); err != nil {
		return segmentRef{}, err
	}
	return segmentRef{segment: item, Tag: tag}, nil
}

func validateSegment(tag string, item segment, manifest manifestRef) error {
	if item.Snapshot == "" || item.System == "" || item.CreatedAt.IsZero() {
		return fmt.Errorf("segment metadata is missing snapshot, system, or createdAt")
	}
	if !strings.HasPrefix(tag, segmentTagPrefix(item.Snapshot, item.System)) {
		return fmt.Errorf("tag %q does not match metadata snapshot %q and system %q", tag, item.Snapshot, item.System)
	}
	if len(item.Entries) != len(manifest.NARLayers) {
		return fmt.Errorf("segment has %d entries but %d NAR layers", len(item.Entries), len(manifest.NARLayers))
	}
	for hash, entry := range item.Entries {
		actualHash, err := storeHash(entry.StorePath)
		if err != nil {
			return fmt.Errorf("entry %s has invalid store path: %w", hash, err)
		}
		if actualHash != hash {
			return fmt.Errorf("entry key %q does not match store path hash %q", hash, actualHash)
		}
		if path.Clean(entry.NARURL) != entry.NARURL || !strings.HasPrefix(entry.NARURL, "nar/") {
			return fmt.Errorf("entry %s has invalid NAR URL %q", hash, entry.NARURL)
		}
		narDigest, err := digest.Parse(entry.NARDigest)
		if err != nil || narDigest.Algorithm() != digest.SHA256 || entry.NARSize <= 0 {
			return fmt.Errorf("entry %s has invalid NAR digest or size", hash)
		}
		layer, ok := manifest.NARLayers[hash]
		if !ok || layer.MediaType != narMediaType || layer.Annotations[storeHashAnnotation] != hash ||
			layer.Digest != narDigest || layer.Size != entry.NARSize {
			return fmt.Errorf("entry %s does not match its manifest NAR layer", hash)
		}
		info, err := narinfo.Parse(strings.NewReader(entry.NARInfo))
		if err != nil {
			return fmt.Errorf("entry %s has invalid narinfo: %w", hash, err)
		}
		if err := info.Check(); err != nil {
			return fmt.Errorf("entry %s has invalid narinfo: %w", hash, err)
		}
		if info.StorePath != entry.StorePath || info.URL != entry.NARURL ||
			info.FileHash == nil || info.FileHash.Algo().String() != digest.SHA256.String() ||
			digest.NewDigestFromEncoded(digest.SHA256, digest.SHA256.Encode(info.FileHash.Digest())).String() != entry.NARDigest ||
			int64(info.FileSize) != entry.NARSize {
			return fmt.Errorf("entry %s metadata does not match narinfo", hash)
		}
	}
	return nil
}

func (client *registryClient) loadEntries(ctx context.Context, tagPrefix string) (map[string]cacheEntry, error) {
	segments, err := client.listSegments(ctx, tagPrefix)
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

func (client *registryClient) blobReader(ctx context.Context, digest string) (io.ReadCloser, error) {
	_, reader, err := client.repo.Blobs().FetchReference(ctx, digest)
	return reader, err
}

const narUploadConcurrency = 8

func (client *registryClient) pushSegment(ctx context.Context, state snapshot, packageKey string, entries map[string]cacheEntry) error {
	runID := os.Getenv("GITHUB_RUN_ID")
	if runID == "" {
		runID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	item := segment{
		Version:          segmentVersion,
		Snapshot:         state.ID,
		RepositoryCommit: state.RepositoryCommit,
		System:           state.System,
		PackageKey:       packageKey,
		RunID:            runID,
		CreatedAt:        time.Now().UTC(),
		Channels:         state.Channels,
		Entries:          entries,
	}
	metadata, err := json.Marshal(item)
	if err != nil {
		return err
	}
	metadataDescriptor := content.NewDescriptorFromBytes(segmentMediaType, metadata)
	err = client.repo.Blobs().Push(ctx, metadataDescriptor, bytes.NewReader(metadata))
	if err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return err
	}

	hashes := slices.Sorted(maps.Keys(entries))
	layers := make([]ocispec.Descriptor, 1, len(entries)+1)
	layers[0] = metadataDescriptor
	uploaded := make([]bool, len(hashes))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(narUploadConcurrency)
	for index, hash := range hashes {
		entry := entries[hash]
		descriptor := ocispec.Descriptor{
			MediaType: narMediaType,
			Digest:    digest.Digest(entry.NARDigest),
			Size:      entry.NARSize,
		}
		descriptor.Annotations = map[string]string{storeHashAnnotation: hash}
		layers = append(layers, descriptor)
		group.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return err
			}
			pushed, err := client.pushFile(groupCtx, descriptor, entry.NARPath)
			if err != nil {
				return fmt.Errorf("push NAR for %s: %w", hash, err)
			}
			uploaded[index] = pushed
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	var uploadedCount int
	for _, pushed := range uploaded {
		if pushed {
			uploadedCount++
		}
	}
	log.Printf("pushed %d NAR blob(s), skipped %d already present", uploadedCount, len(uploaded)-uploadedCount)

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
	tag := client.segmentTag(state, runID)
	if err := client.repo.PushReference(ctx, manifestDescriptor, bytes.NewReader(manifest), tag); err != nil {
		return fmt.Errorf("publish segment %s: %w", tag, err)
	}
	log.Printf("published segment %s for package %s (%d entries)", tag, packageKey, len(entries))
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
	defer func() { _ = file.Close() }()
	if err := client.repo.Blobs().Push(ctx, descriptor, file); err != nil {
		if errors.Is(err, errdef.ErrAlreadyExists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (client *registryClient) segmentTag(state snapshot, runID string) string {
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
