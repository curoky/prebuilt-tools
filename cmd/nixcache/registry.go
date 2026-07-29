package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"
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
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		username := os.Getenv("GITHUB_ACTOR")
		if username == "" {
			username = "token"
		}
		credential = auth.StaticCredential(repo.Reference.Registry, auth.Credential{
			Username: username,
			Password: token,
		})
	} else {
		store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
		if err != nil {
			return nil, fmt.Errorf("load registry credentials: %w", err)
		}
		credential = credentials.Credential(store)
	}
	client := *auth.DefaultClient
	client.Credential = credential
	repo.Client = &client
	return &registryClient{repo: repo}, nil
}

func (client *registryClient) listSegments() ([]segment, error) {
	tags, err := registry.Tags(context.Background(), client.repo)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) || isNameUnknown(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list cache segments: %w", err)
	}

	segments := make([]segment, 0, len(tags))
	for _, tag := range tags {
		if !strings.HasPrefix(tag, segmentPrefix) {
			continue
		}
		item, err := client.getSegment(tag)
		if err != nil {
			return nil, fmt.Errorf("load segment %s: %w", tag, err)
		}
		segments = append(segments, item)
	}
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].CreatedAt.Before(segments[j].CreatedAt)
	})
	return segments, nil
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

func (client *registryClient) getSegment(tag string) (segment, error) {
	_, reader, err := client.repo.FetchReference(context.Background(), tag)
	if err != nil {
		return segment{}, err
	}
	defer reader.Close()

	var manifest ocispec.Manifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return segment{}, err
	}
	if len(manifest.Layers) == 0 {
		return segment{}, fmt.Errorf("segment has no metadata layer")
	}
	metadata := manifest.Layers[0]
	if metadata.MediaType != segmentMediaType {
		return segment{}, fmt.Errorf("unexpected metadata media type %q", metadata.MediaType)
	}
	reader, err = client.repo.Fetch(context.Background(), metadata)
	if err != nil {
		return segment{}, err
	}
	defer reader.Close()

	var item segment
	if err := json.NewDecoder(reader).Decode(&item); err != nil {
		return segment{}, fmt.Errorf("decode segment metadata: %w", err)
	}
	if item.Version != segmentVersion {
		return segment{}, fmt.Errorf("unsupported segment version %d", item.Version)
	}
	return item, nil
}

func (client *registryClient) loadEntries() (map[string]cacheEntry, error) {
	segments, err := client.listSegments()
	if err != nil {
		return nil, err
	}
	entries := make(map[string]cacheEntry)
	for _, item := range segments {
		for hash, entry := range item.Entries {
			entries[hash] = entry
		}
	}
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
	for _, hash := range hashes {
		entry := entries[hash]
		descriptor := ocispec.Descriptor{
			MediaType: narMediaType,
			Digest:    digest.Digest(entry.NARDigest),
			Size:      entry.NARSize,
		}
		descriptor.Annotations = map[string]string{"org.nixos.store.hash": hash}
		if err := client.pushFile(ctx, descriptor, entry.NARPath); err != nil {
			return err
		}
		layers = append(layers, descriptor)
	}

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
	return nil
}

func (client *registryClient) pushFile(ctx context.Context, descriptor ocispec.Descriptor, path string) error {
	exists, err := client.repo.Blobs().Exists(ctx, descriptor)
	if err != nil || exists {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return client.repo.Blobs().Push(ctx, descriptor, file)
}

func (client *registryClient) segmentTag(state snapshot) string {
	snapshotID := strings.TrimPrefix(state.ID, "sha256:")
	if len(snapshotID) > 16 {
		snapshotID = snapshotID[:16]
	}
	runID := os.Getenv("GITHUB_RUN_ID")
	if runID == "" {
		runID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%s%s-%s-%s-%s", segmentPrefix, snapshotID, state.System, runID, rand.Text())
}
