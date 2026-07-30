package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nix-community/go-nix/pkg/storepath"
)

const (
	cacheRepository = "ghcr.io/curoky/standalone-binaries-cache"
	segmentPrefix   = "v1-"
	segmentVersion  = 1

	segmentMediaType = "application/vnd.curoky.nixcache.segment.v1+json"
	narMediaType     = "application/vnd.curoky.nixcache.nar.v1"
)

type cacheEntry struct {
	StorePath string `json:"storePath"`
	NARURL    string `json:"narUrl"`
	NARDigest string `json:"narDigest"`
	NARSize   int64  `json:"narSize"`
	NARInfo   string `json:"narInfo"`
	NARPath   string `json:"-"`
}

type segment struct {
	Version          int                   `json:"version"`
	Snapshot         string                `json:"snapshot"`
	RepositoryCommit string                `json:"repositoryCommit,omitempty"`
	System           string                `json:"system"`
	PackageKey       string                `json:"packageKey,omitempty"`
	RunID            string                `json:"runId,omitempty"`
	CreatedAt        time.Time             `json:"createdAt"`
	Channels         map[string]string     `json:"channels"`
	Entries          map[string]cacheEntry `json:"entries"`
}

type snapshot struct {
	ID               string
	RepositoryCommit string
	System           string
	Channels         map[string]string
}

type flakeLock struct {
	Nodes map[string]struct {
		Locked struct {
			Rev string `json:"rev"`
		} `json:"locked"`
	} `json:"nodes"`
}

func loadSnapshot(repoRoot string) (snapshot, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "flake.lock"))
	if err != nil {
		return snapshot{}, fmt.Errorf("read flake.lock: %w", err)
	}

	var lock flakeLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return snapshot{}, fmt.Errorf("parse flake.lock: %w", err)
	}

	channels := make(map[string]string)
	for name, node := range lock.Nodes {
		if strings.HasPrefix(name, "nixpkgs-") && node.Locked.Rev != "" {
			channels[name] = node.Locked.Rev
		}
	}
	if len(channels) == 0 {
		return snapshot{}, fmt.Errorf("flake.lock contains no nixpkgs channel revisions")
	}

	sum := sha256.Sum256(data)
	return snapshot{
		ID:               "sha256:" + hex.EncodeToString(sum[:]),
		RepositoryCommit: os.Getenv("GITHUB_SHA"),
		System:           currentSystem(),
		Channels:         channels,
	}, nil
}

func currentSystem() string {
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return "x86_64-linux"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "aarch64-darwin"
	default:
		return runtime.GOARCH + "-" + runtime.GOOS
	}
}

func storeHash(path string) (string, error) {
	storePath, err := storepath.FromAbsolutePath(path)
	if err != nil {
		return "", err
	}
	return strings.SplitN(storePath.String(), "-", 2)[0], nil
}
