package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// pushTestSegment publishes a one-entry segment for the given snapshot, system,
// and run id so prune/size tests can build up a realistic tag set.
func pushTestSegment(t *testing.T, client *registryClient, snapshotID, system, packageKey, runID, narBody string) {
	t.Helper()
	t.Setenv("GITHUB_RUN_ID", runID)

	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	state := snapshot{
		ID:       snapshotID,
		System:   system,
		Channels: map[string]string{"nixpkgs-unstable": snapshotID},
	}
	entry := testCacheEntry(t, hash, "root", []byte(narBody))
	if err := client.pushSegment(context.Background(), state, packageKey, map[string]cacheEntry{hash: entry}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
}

func ref(snapshotID, system, packageKey, tag string, created int64) segmentRef {
	return segmentRef{
		segment: segment{
			Snapshot:   snapshotID,
			System:     system,
			PackageKey: packageKey,
			CreatedAt:  time.Unix(0, created),
		},
		Tag: tag,
	}
}

// refAt builds a segmentRef with an explicit wall-clock CreatedAt so package
// retention window tests can place segments inside or outside the day window.
func refAt(snapshotID, system, packageKey, tag string, created time.Time) segmentRef {
	return segmentRef{
		segment: segment{
			Snapshot:   snapshotID,
			System:     system,
			PackageKey: packageKey,
			CreatedAt:  created,
		},
		Tag: tag,
	}
}

func TestKeptSnapshotsKeepsNewestPerSystem(t *testing.T) {
	segments := []segmentRef{
		ref("snap-1", "x86_64-linux", "pkg", "t1", 1),
		ref("snap-2", "x86_64-linux", "pkg", "t2", 2),
		ref("snap-3", "x86_64-linux", "pkg", "t3", 3),
		ref("snap-4", "aarch64-darwin", "pkg", "t4", 4),
		ref("snap-5", "aarch64-darwin", "pkg", "t5", 5),
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
		ref("snap-old", "x86_64-linux", "pkg", "old-a", 1),
		ref("snap-new", "x86_64-linux", "pkg", "new", 5),
		ref("snap-old", "x86_64-linux", "pkg", "old-b", 9),
	}
	kept := keptSnapshots(segments, 1)
	if !kept["x86_64-linux"]["snap-old"] || kept["x86_64-linux"]["snap-new"] {
		t.Fatalf("snapshot recency should use its newest tag: %v", kept["x86_64-linux"])
	}
}

func TestPruneDryRunDeletesNothing(t *testing.T) {
	client := testRegistryClient(t)
	pushTestSegment(t, client, "sha256:1111111111111111000000000000000000000000000000000000000000000000", "x86_64-linux", "one", "1", "one")
	pushTestSegment(t, client, "sha256:2222222222222222000000000000000000000000000000000000000000000000", "x86_64-linux", "two", "2", "two")
	pushTestSegment(t, client, "sha256:3333333333333333000000000000000000000000000000000000000000000000", "x86_64-linux", "three", "3", "three")

	if err := pruneCache(context.Background(), client, 1, 2, 2, true); err != nil {
		t.Fatal(err)
	}
	segments, err := client.listSegments(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 3 {
		t.Fatalf("dry-run must not delete, got %d tags", len(segments))
	}
}

func TestSegmentsToDeleteKeepsLatestPerPackage(t *testing.T) {
	segments := []segmentRef{
		ref("snapshot", "x86_64-linux", "alpha", "alpha-old", 1),
		ref("snapshot", "x86_64-linux", "beta", "beta-only", 2),
		ref("snapshot", "x86_64-linux", "alpha", "alpha-new", 3),
	}
	// retainDays=0 disables the window, so only the newest segment per package
	// survives via the packageKeep=1 fallback.
	toDelete, retained := segmentsToDelete(segments, 1, 0, 1)
	if len(toDelete) != 1 || toDelete[0].Tag != "alpha-old" {
		t.Fatalf("toDelete=%v", toDelete)
	}
	if retained != 2 {
		t.Fatalf("retained=%d", retained)
	}
}

func TestSegmentsToDeleteKeepsPackagesMissingFromIncrementalRun(t *testing.T) {
	segments := []segmentRef{
		ref("snapshot", "x86_64-linux", "alpha", "run-1-alpha", 1),
		ref("snapshot", "x86_64-linux", "beta", "run-1-beta", 2),
		ref("snapshot", "x86_64-linux", "alpha", "run-2-alpha", 3),
	}
	toDelete, _ := segmentsToDelete(segments, 1, 0, 1)
	if len(toDelete) != 1 || toDelete[0].Tag != "run-1-alpha" {
		t.Fatalf("incremental run must retain beta: %v", toDelete)
	}
}

func TestSegmentsToDeleteKeepsLegacySegments(t *testing.T) {
	segments := []segmentRef{
		ref("snapshot", "x86_64-linux", "", "legacy-a", 1),
		ref("snapshot", "x86_64-linux", "", "legacy-b", 2),
	}
	toDelete, retained := segmentsToDelete(segments, 1, 2, 2)
	if len(toDelete) != 0 || retained != 2 {
		t.Fatalf("toDelete=%v retained=%d", toDelete, retained)
	}
}

// TestSegmentsToDeleteKeepsPackageSegmentsWithinDayWindow verifies that every
// segment pushed within retainDays of a package's newest segment survives, while
// older segments outside the window are deleted.
func TestSegmentsToDeleteKeepsPackageSegmentsWithinDayWindow(t *testing.T) {
	now := time.Now()
	segments := []segmentRef{
		refAt("snapshot", "x86_64-linux", "alpha", "alpha-now", now),
		refAt("snapshot", "x86_64-linux", "alpha", "alpha-1d", now.Add(-24*time.Hour)),
		refAt("snapshot", "x86_64-linux", "alpha", "alpha-3d", now.Add(-72*time.Hour)),
	}
	// keep=1 snapshot, 2-day window, fallback 1. The 3-day-old segment is
	// outside the window and beyond the fallback, so it is deleted.
	toDelete, retained := segmentsToDelete(segments, 1, 2, 1)
	if len(toDelete) != 1 || toDelete[0].Tag != "alpha-3d" {
		t.Fatalf("toDelete=%v", toDelete)
	}
	if retained != 2 {
		t.Fatalf("retained=%d", retained)
	}
}

// TestSegmentsToDeleteFallsBackToPackageKeep verifies that when the day window
// holds fewer than packageKeep segments, the newest packageKeep segments are
// retained regardless of age.
func TestSegmentsToDeleteFallsBackToPackageKeep(t *testing.T) {
	now := time.Now()
	segments := []segmentRef{
		refAt("snapshot", "x86_64-linux", "alpha", "alpha-10d", now.Add(-240*time.Hour)),
		refAt("snapshot", "x86_64-linux", "alpha", "alpha-11d", now.Add(-264*time.Hour)),
		refAt("snapshot", "x86_64-linux", "alpha", "alpha-12d", now.Add(-288*time.Hour)),
	}
	// All segments are older than the 2-day window relative to now, but the
	// window is anchored on the group's newest segment (10d), so 11d falls in.
	// 12d is outside the window; fallback packageKeep=2 still retains it.
	toDelete, retained := segmentsToDelete(segments, 1, 2, 2)
	if len(toDelete) != 1 || toDelete[0].Tag != "alpha-12d" {
		t.Fatalf("toDelete=%v", toDelete)
	}
	if retained != 2 {
		t.Fatalf("retained=%d", retained)
	}
}

func TestCacheSizeDeduplicatesBlobs(t *testing.T) {
	client := testRegistryClient(t)
	// Two segments sharing the identical NAR body: the shared blob counts once.
	pushTestSegment(t, client, "sha256:1111111111111111000000000000000000000000000000000000000000000000", "x86_64-linux", "one", "1", "shared")
	pushTestSegment(t, client, "sha256:2222222222222222000000000000000000000000000000000000000000000000", "x86_64-linux", "two", "2", "shared")

	total, err := cacheSize(context.Background(), client)
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
	if err := pruneCache(context.Background(), client, 0, 2, 2, false); err == nil {
		t.Fatal("expected error for keep=0")
	}
}

type fakeVersionDeleter struct {
	deleted []int64
	err     error
}

func (fake *fakeVersionDeleter) deleteVersion(_ context.Context, versionID int64) error {
	if fake.err != nil {
		return fake.err
	}
	fake.deleted = append(fake.deleted, versionID)
	return nil
}

type cancelingVersionDeleter struct {
	started  chan int64
	canceled chan struct{}
	fail     chan struct{}
}

func (fake *cancelingVersionDeleter) deleteVersion(ctx context.Context, versionID int64) error {
	fake.started <- versionID
	if versionID == 1 {
		<-fake.fail
		return errors.New("delete failed")
	}
	<-ctx.Done()
	fake.canceled <- struct{}{}
	return ctx.Err()
}

func TestDeleteVersionsRejectsMissingMappingBeforeDeleting(t *testing.T) {
	deleter := &fakeVersionDeleter{}
	segments := []segmentRef{{Tag: "one"}, {Tag: "missing"}}
	deleted, err := deleteVersions(context.Background(), deleter, segments, map[string]int64{"one": 1})
	if err == nil {
		t.Fatal("expected missing mapping error")
	}
	if deleted != 0 || len(deleter.deleted) != 0 {
		t.Fatalf("deleted=%d calls=%v", deleted, deleter.deleted)
	}
}

func TestDeleteVersionsReturnsDeleteError(t *testing.T) {
	deleter := &fakeVersionDeleter{err: errors.New("delete failed")}
	segments := []segmentRef{{Tag: "one"}}
	deleted, err := deleteVersions(context.Background(), deleter, segments, map[string]int64{"one": 1})
	if err == nil || deleted != 0 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
}

func TestDeleteVersionsCancelsRemainingRequests(t *testing.T) {
	const activeRequests = segmentDeleteConcurrency - 1
	deleter := &cancelingVersionDeleter{
		started:  make(chan int64, segmentDeleteConcurrency),
		canceled: make(chan struct{}, activeRequests),
		fail:     make(chan struct{}),
	}
	segments := make([]segmentRef, segmentDeleteConcurrency+2)
	versionByTag := make(map[string]int64, len(segments))
	for index := range segments {
		tag := fmt.Sprintf("segment-%d", index+1)
		segments[index] = segmentRef{Tag: tag}
		versionByTag[tag] = int64(index + 1)
	}
	result := make(chan error, 1)
	go func() {
		_, err := deleteVersions(context.Background(), deleter, segments, versionByTag)
		result <- err
	}()
	for range segmentDeleteConcurrency {
		select {
		case <-deleter.started:
		case <-time.After(time.Second):
			t.Fatal("delete requests did not reach concurrency limit")
		}
	}
	close(deleter.fail)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected delete error")
		}
	case <-time.After(time.Second):
		t.Fatal("delete batch did not return after first error")
	}
	for range activeRequests {
		select {
		case <-deleter.canceled:
		case <-time.After(time.Second):
			t.Fatal("active delete request was not canceled")
		}
	}
	select {
	case versionID := <-deleter.started:
		t.Fatalf("queued delete request %d started after cancellation", versionID)
	default:
	}
}
