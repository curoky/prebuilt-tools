package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultListen = "127.0.0.1:37515"
	refreshEvery  = 5 * time.Minute
	nixCacheInfo  = "StoreDir: /nix/store\nWantMassQuery: 1\nPriority: 30\n"
	narInfoSuffix = ".narinfo"
)

type cacheIndex struct {
	client *registryClient

	mu      sync.RWMutex
	entries map[string]cacheEntry
	nars    map[string]cacheEntry
}

func newCacheIndex(client *registryClient) *cacheIndex {
	return &cacheIndex{
		client:  client,
		entries: make(map[string]cacheEntry),
		nars:    make(map[string]cacheEntry),
	}
}

func (index *cacheIndex) refresh() (int, error) {
	entries, err := index.client.loadEntries()
	if err != nil {
		return 0, err
	}
	nars := make(map[string]cacheEntry, len(entries))
	for _, entry := range entries {
		nars[entry.NARURL] = entry
	}

	index.mu.Lock()
	index.entries = entries
	index.nars = nars
	index.mu.Unlock()
	return len(entries), nil
}

func (index *cacheIndex) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(request.URL.Path, "/")
	switch {
	case path == "nix-cache-info":
		serveBytes(writer, request, "text/x-nix-cache-info", nixCacheInfo)
	case strings.HasSuffix(path, narInfoSuffix):
		hash := strings.TrimSuffix(path, narInfoSuffix)
		index.mu.RLock()
		entry, ok := index.entries[hash]
		index.mu.RUnlock()
		if !ok {
			http.NotFound(writer, request)
			return
		}
		serveBytes(writer, request, "text/x-nix-narinfo", entry.NARInfo)
	case strings.HasPrefix(path, "nar/"):
		index.mu.RLock()
		entry, ok := index.nars[path]
		index.mu.RUnlock()
		if !ok {
			http.NotFound(writer, request)
			return
		}
		if request.Method == http.MethodHead {
			setNARHeaders(writer, entry)
			writer.WriteHeader(http.StatusOK)
			return
		}
		reader, err := index.client.blobReader(entry.NARDigest)
		if err != nil {
			log.Printf("read cache blob %s: %v", entry.NARDigest, err)
			http.Error(writer, "read cache blob", http.StatusBadGateway)
			return
		}
		defer reader.Close()
		setNARHeaders(writer, entry)
		if _, err := io.Copy(writer, reader); err != nil {
			log.Printf("stream %s: %v", path, err)
		}
	default:
		http.NotFound(writer, request)
	}
}

func serveBytes(writer http.ResponseWriter, request *http.Request, contentType, body string) {
	writer.Header().Set("Content-Type", contentType)
	http.ServeContent(writer, request, "", time.Time{}, bytes.NewReader([]byte(body)))
}

func setNARHeaders(writer http.ResponseWriter, entry cacheEntry) {
	writer.Header().Set("Content-Type", "application/x-nix-nar")
	writer.Header().Set("Content-Length", fmt.Sprintf("%d", entry.NARSize))
	writer.Header().Set("ETag", `"`+entry.NARDigest+`"`)
}

func serveCache(client *registryClient) error {
	index := newCacheIndex(client)

	// Load the index synchronously before opening the listener so that a
	// successful nix-cache-info readiness probe implies the entries are already
	// queryable. An async first load would let CI start path-info lookups while
	// the index is still empty, turning every cache hit into a spurious miss.
	// A missing cache repository is not an error (listSegments maps NAME_UNKNOWN
	// to an empty set), so a fresh repo still starts with an empty index.
	count, err := index.refresh()
	if err != nil {
		return fmt.Errorf("initial cache index load: %w", err)
	}
	log.Printf("loaded cache index: %d entries", count)

	listener, err := net.Listen("tcp", defaultListen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", defaultListen, err)
	}

	go func() {
		ticker := time.NewTicker(refreshEvery)
		defer ticker.Stop()
		for range ticker.C {
			if count, err := index.refresh(); err != nil {
				log.Printf("refresh cache index: %v", err)
			} else {
				log.Printf("refreshed cache index: %d entries", count)
			}
		}
	}()

	log.Printf("serving Nix cache at http://%s", defaultListen)
	server := &http.Server{
		Handler:           withAccessLog(http.HandlerFunc(index.serveHTTP)),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	return server.Serve(listener)
}

// statusRecorder captures the response status so the access log can report it;
// net/http does not expose the code written to a ResponseWriter otherwise.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

// withAccessLog logs one line per request so cache hits and misses are visible
// in CI logs, which is the main signal for confirming the substituter works.
func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(recorder, request)
		log.Printf("%s %s -> %d (%s)", request.Method, request.URL.Path, recorder.status, time.Since(start).Round(time.Millisecond))
	})
}
