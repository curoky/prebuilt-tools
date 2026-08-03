package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeStorePath(t *testing.T) {
	const storePath = "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-fixture"
	entry := testCacheEntry(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "fixture", []byte("nar"))

	tests := []struct {
		name    string
		handler http.HandlerFunc
		hit     bool
		wantErr string
	}{
		{
			name: "hit",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.narinfo" {
					http.NotFound(writer, request)
					return
				}
				_, _ = writer.Write([]byte(entry.NARInfo))
			},
			hit: true,
		},
		{
			name: "miss",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				http.NotFound(writer, request)
			},
		},
		{
			name: "service failure",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				http.Error(writer, "cache unavailable", http.StatusServiceUnavailable)
			},
			wantErr: "503 Service Unavailable",
		},
		{
			name: "wrong store path",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				_, _ = writer.Write([]byte(strings.Replace(
					entry.NARInfo,
					storePath,
					"/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-other",
					1,
				)))
			},
			wantErr: "cache returned StorePath",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()

			hit, err := probeStorePath(context.Background(), server.Client(), server.URL, storePath)
			if hit != test.hit {
				t.Fatalf("hit=%t want %t", hit, test.hit)
			}
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("err=%v want substring %q", err, test.wantErr)
			}
		})
	}
}
