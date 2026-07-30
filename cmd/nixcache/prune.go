package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/opencontainers/go-digest"
)

// cacheSize reports the deduplicated on-registry byte size of the cache: every
// distinct layer digest referenced by any segment manifest is counted once,
// because segments share NAR blobs by digest.
func cacheSize(client *registryClient) (int64, error) {
	segments, err := client.listSegments("")
	if err != nil {
		return 0, err
	}
	seen := make(map[digest.Digest]int64)
	for _, ref := range segments {
		for layer, size := range ref.LayerSizes {
			seen[layer] = size
		}
	}
	var total int64
	for _, size := range seen {
		total += size
	}
	return total, nil
}

// pruneCache trims the cache along two axes. First it keeps only the newest
// `keep` snapshots per system and drops every segment of older snapshots
// (retention by snapshot recency, not channel revision ordering, since Nix
// channel revisions are git hashes with no total order). Then, within each
// kept snapshot+system, it keeps only the newest `keepTags` tags: a snapshot
// that never changes accumulates one tag per CI run, so without this cap the
// cache grows unbounded and serve must fetch hundreds of duplicate segments.
func pruneCache(client *registryClient, keep, keepTags int, dryRun bool) error {
	if keep < 1 {
		return fmt.Errorf("keep must be at least 1, got %d", keep)
	}
	if keepTags < 1 {
		return fmt.Errorf("keep-tags must be at least 1, got %d", keepTags)
	}
	segments, err := client.listSegments("")
	if err != nil {
		return err
	}
	if len(segments) == 0 {
		log.Printf("no cache segments found")
		return nil
	}

	kept := keptSnapshots(segments, keep)
	keptTag := keptTags(segments, keepTags)

	toDelete := make([]segmentRef, 0)
	var retained int
	for _, ref := range segments {
		if kept[ref.System][ref.Snapshot] && keptTag[ref.Tag] {
			retained++
			continue
		}
		toDelete = append(toDelete, ref)
	}

	if dryRun {
		for _, ref := range toDelete {
			log.Printf("would delete segment %s (snapshot %s, %s)", ref.Tag, shortSnapshot(ref.Snapshot), ref.System)
		}
		log.Printf("prune dry-run: would delete %d segment(s), retain %d", len(toDelete), retained)
		return nil
	}
	if len(toDelete) == 0 {
		log.Printf("prune complete: deleted 0 segment(s), retained %d", retained)
		return nil
	}

	// GHCR does not support OCI manifest deletion, so map each tag to its
	// GitHub package version id and delete via the Packages REST API.
	ghcr, err := newGHCRClient(client.repo.Reference.Repository)
	if err != nil {
		return err
	}
	ctx := context.Background()
	versionByTag, err := ghcr.versionsByTag(ctx)
	if err != nil {
		return fmt.Errorf("list package versions: %w", err)
	}

	if err := deleteVersions(ctx, ghcr, toDelete, versionByTag); err != nil {
		return err
	}
	log.Printf("prune complete: deleted %d segment(s), retained %d", len(toDelete), retained)
	return nil
}

// segmentDeleteConcurrency bounds how many package versions are deleted in
// parallel. Deletions are independent REST calls; a large prune faces hundreds
// of tags and serial deletion takes minutes. Kept modest to stay clear of the
// GitHub API secondary rate limit.
const segmentDeleteConcurrency = 8

// deleteVersions deletes every segment in toDelete concurrently, capped at
// segmentDeleteConcurrency, and logs one summary line per 100 deletions rather
// than one per tag. Tags without a matching package version are skipped. The
// first error cancels the batch.
func deleteVersions(ctx context.Context, ghcr *ghcrClient, toDelete []segmentRef, versionByTag map[string]int64) error {
	results := make(chan error, len(toDelete))
	sem := make(chan struct{}, segmentDeleteConcurrency)
	var wg sync.WaitGroup
	for _, ref := range toDelete {
		versionID, ok := versionByTag[ref.Tag]
		if !ok {
			log.Printf("skip segment %s: no matching package version", ref.Tag)
			results <- nil
			continue
		}
		wg.Add(1)
		go func(ref segmentRef, versionID int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := ghcr.deleteVersion(ctx, versionID); err != nil {
				results <- fmt.Errorf("delete segment %s (version %d): %w", ref.Tag, versionID, err)
				return
			}
			results <- nil
		}(ref, versionID)
	}
	go func() { wg.Wait(); close(results) }()

	var deleteErr error
	var done int
	for err := range results {
		if err != nil {
			if deleteErr == nil {
				deleteErr = err
			}
			continue
		}
		done++
		if done%100 == 0 || done == len(toDelete) {
			log.Printf("deleted %d/%d segments", done, len(toDelete))
		}
	}
	return deleteErr
}

// keptSnapshots returns, per system, the set of snapshot IDs to keep: the keep
// most recent snapshots ranked by their newest segment's creation time.
func keptSnapshots(segments []segmentRef, keep int) map[string]map[string]bool {
	newest := make(map[string]map[string]int64) // system -> snapshot -> newest unix nanos
	for _, ref := range segments {
		bySnapshot := newest[ref.System]
		if bySnapshot == nil {
			bySnapshot = make(map[string]int64)
			newest[ref.System] = bySnapshot
		}
		if created := ref.CreatedAt.UnixNano(); created > bySnapshot[ref.Snapshot] {
			bySnapshot[ref.Snapshot] = created
		}
	}

	kept := make(map[string]map[string]bool, len(newest))
	for system, bySnapshot := range newest {
		type snapshotAge struct {
			id      string
			created int64
		}
		ranked := make([]snapshotAge, 0, len(bySnapshot))
		for id, created := range bySnapshot {
			ranked = append(ranked, snapshotAge{id: id, created: created})
		}
		sort.Slice(ranked, func(i, j int) bool {
			return ranked[i].created > ranked[j].created
		})
		set := make(map[string]bool)
		for i := 0; i < len(ranked) && i < keep; i++ {
			set[ranked[i].id] = true
		}
		kept[system] = set
	}
	return kept
}

// keptTags returns the set of tags to keep: within each system+snapshot group,
// the keepTags most recently created tags. Ties on CreatedAt are broken by tag
// name so the selection is deterministic.
func keptTags(segments []segmentRef, keepTags int) map[string]bool {
	type group struct{ system, snapshot string }
	byGroup := make(map[group][]segmentRef)
	for _, ref := range segments {
		key := group{ref.System, ref.Snapshot}
		byGroup[key] = append(byGroup[key], ref)
	}

	kept := make(map[string]bool)
	for _, refs := range byGroup {
		sort.Slice(refs, func(i, j int) bool {
			if !refs[i].CreatedAt.Equal(refs[j].CreatedAt) {
				return refs[i].CreatedAt.After(refs[j].CreatedAt)
			}
			return refs[i].Tag > refs[j].Tag
		})
		for i := 0; i < len(refs) && i < keepTags; i++ {
			kept[refs[i].Tag] = true
		}
	}
	return kept
}

func shortSnapshot(id string) string {
	trimmed := id
	if len(trimmed) > 23 { // "sha256:" + 16
		trimmed = trimmed[:23]
	}
	return trimmed
}

// humanSize renders a byte count using binary units for log output.
func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
