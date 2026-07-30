package main

import (
	"fmt"
	"log"
	"sort"

	"github.com/opencontainers/go-digest"
)

// cacheSize reports the deduplicated on-registry byte size of the cache: every
// distinct layer digest referenced by any segment manifest is counted once,
// because segments share NAR blobs by digest.
func cacheSize(client *registryClient) (int64, error) {
	segments, err := client.listSegments()
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

// pruneCache keeps the newest keep snapshots per system and deletes the segment
// manifests of older snapshots. A snapshot may map to several tags (one per CI
// run); all tags of a kept snapshot are kept, all tags of a dropped snapshot are
// deleted. Retention is by snapshot recency, not by channel revision ordering,
// because Nix channel revisions are git hashes with no total order.
func pruneCache(client *registryClient, keep int, dryRun bool) error {
	if keep < 1 {
		return fmt.Errorf("keep must be at least 1, got %d", keep)
	}
	segments, err := client.listSegments()
	if err != nil {
		return err
	}
	if len(segments) == 0 {
		log.Printf("no cache segments found")
		return nil
	}

	kept := keptSnapshots(segments, keep)
	var deleted, retained int
	for _, ref := range segments {
		if kept[ref.System][ref.Snapshot] {
			retained++
			continue
		}
		if dryRun {
			log.Printf("would delete segment %s (snapshot %s, %s)", ref.Tag, shortSnapshot(ref.Snapshot), ref.System)
			deleted++
			continue
		}
		if err := client.deleteSegment(ref); err != nil {
			return fmt.Errorf("delete segment %s: %w", ref.Tag, err)
		}
		log.Printf("deleted segment %s (snapshot %s, %s)", ref.Tag, shortSnapshot(ref.Snapshot), ref.System)
		deleted++
	}
	log.Printf("prune complete: deleted %d segment(s), retained %d", deleted, retained)
	return nil
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
