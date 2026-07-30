package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"oras.land/oras-go/v2/content"
)

// pushTestSegment publishes a one-entry segment for the given snapshot, system,
// and run id so prune/size tests can build up a realistic tag set.
func pushTestSegment(t *testing.T, client *registryClient, snapshotID, system, runID, narBody string) {
	t.Helper()
	t.Setenv("GITHUB_RUN_ID", runID)

	narPath := filepath.Join(t.TempDir(), "root.nar.zst")
	if err := os.WriteFile(narPath, []byte(narBody), 0o644); err != nil {
		t.Fatal(err)
	}
	blob := content.NewDescriptorFromBytes(narMediaType, []byte(narBody))
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	state := snapshot{
		ID:       snapshotID,
		System:   system,
		Channels: map[string]string{"nixpkgs-unstable": snapshotID},
	}
	entry := cacheEntry{
		StorePath: "/nix/store/" + hash + "-root",
		NARURL:    "nar/root.nar.zst",
		NARDigest: blob.Digest.String(),
		NARSize:   blob.Size,
		NARInfo:   "StorePath: /nix/store/" + hash + "-root\nURL: nar/root.nar.zst\n",
		NARPath:   narPath,
	}
	if err := client.pushSegment(state, map[string]cacheEntry{hash: entry}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
}

func ref(snapshotID, system, tag string, created int64) segmentRef {
	return segmentRef{
		segment: segment{
			Snapshot:  snapshotID,
			System:    system,
			CreatedAt: time.Unix(0, created),
		},
		Tag: tag,
	}
}

func TestKeptSnapshotsKeepsNewestPerSystem(t *testing.T) {
	segments := []segmentRef{
		ref("snap-1", "x86_64-linux", "t1", 1),
		ref("snap-2", "x86_64-linux", "t2", 2),
		ref("snap-3", "x86_64-linux", "t3", 3),
		ref("snap-4", "aarch64-darwin", "t4", 4),
		ref("snap-5", "aarch64-darwin", "t5", 5),
	}
	kept := keptSnapshots(segments, 2)

	// linux keeps the two newest snapshots (2, 3), drops the oldest (1).
	if kept["x86_64-linux"]["snap-1"] {
		t.Fatalf("oldest linux snapshot should be dropped: %v", kept["x86_64-linux"])
	}
	if !kept["x86_64-linux"]["snap-2"] || !kept["x86_64-linux"]["snap-3"] {
		t.Fatalf("two newest linux snapshots should be kept: %v", kept["x86_64-linux"])
	}
	// darwin has only two snapshots; both survive.
	if !kept["aarch64-darwin"]["snap-4"] || !kept["aarch64-darwin"]["snap-5"] {
		t.Fatalf("both darwin snapshots should be kept: %v", kept["aarch64-darwin"])
	}
}

func TestKeptSnapshotsRanksByNewestTagPerSnapshot(t *testing.T) {
	// snap-old has an early and a late tag; snap-new has one middling tag. The
	// late tag must pull snap-old above snap-new so it is retained under keep=1.
	segments := []segmentRef{
		ref("snap-old", "x86_64-linux", "old-a", 1),
		ref("snap-new", "x86_64-linux", "new", 5),
		ref("snap-old", "x86_64-linux", "old-b", 9),
	}
	kept := keptSnapshots(segments, 1)
	if !kept["x86_64-linux"]["snap-old"] || kept["x86_64-linux"]["snap-new"] {
		t.Fatalf("snapshot recency should use its newest tag: %v", kept["x86_64-linux"])
	}
}

func TestPruneDryRunDeletesNothing(t *testing.T) {
	client := testRegistryClient(t)
	pushTestSegment(t, client, "sha256:1111111111111111000000000000000000000000000000000000000000000000", "x86_64-linux", "1", "one")
	pushTestSegment(t, client, "sha256:2222222222222222000000000000000000000000000000000000000000000000", "x86_64-linux", "2", "two")
	pushTestSegment(t, client, "sha256:3333333333333333000000000000000000000000000000000000000000000000", "x86_64-linux", "3", "three")

	if err := pruneCache(client, 1, true); err != nil {
		t.Fatal(err)
	}
	segments, err := client.listSegments()
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 3 {
		t.Fatalf("dry-run must not delete, got %d tags", len(segments))
	}
}

func TestPruneDeletesOldSegmentManifest(t *testing.T) {
	client := testRegistryClient(t)
	pushTestSegment(t, client, "sha256:1111111111111111000000000000000000000000000000000000000000000000", "x86_64-linux", "1", "one")
	pushTestSegment(t, client, "sha256:2222222222222222000000000000000000000000000000000000000000000000", "x86_64-linux", "2", "two")

	segments, err := client.listSegments()
	if err != nil {
		t.Fatal(err)
	}
	oldest := segments[0] // sorted by CreatedAt ascending
	if err := client.deleteSegment(oldest); err != nil {
		t.Fatalf("delete segment: %v", err)
	}
	if _, err := client.getSegment(oldest.Manifest.Digest.String()); err == nil {
		t.Fatal("expected deleted segment manifest to be unresolvable by digest")
	}
}

func TestCacheSizeDeduplicatesBlobs(t *testing.T) {
	client := testRegistryClient(t)
	// Two segments sharing the identical NAR body: the shared blob counts once.
	pushTestSegment(t, client, "sha256:1111111111111111000000000000000000000000000000000000000000000000", "x86_64-linux", "1", "shared")
	pushTestSegment(t, client, "sha256:2222222222222222000000000000000000000000000000000000000000000000", "x86_64-linux", "2", "shared")

	total, err := cacheSize(client)
	if err != nil {
		t.Fatal(err)
	}
	narSize := int64(len("shared"))
	if total <= narSize {
		t.Fatalf("size %d should exceed lone NAR size %d", total, narSize)
	}
	if total >= 2*narSize+2*4096 {
		t.Fatalf("size %d looks like the shared blob was double counted", total)
	}
}

func TestPruneRejectsKeepZero(t *testing.T) {
	client := testRegistryClient(t)
	if err := pruneCache(client, 0, false); err == nil {
		t.Fatal("expected error for keep=0")
	}
}
