package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSnapshot(t *testing.T) {
	root := t.TempDir()
	body := `{
  "nodes": {
    "nixpkgs-unstable": {"locked": {"rev": "aaaaaaaa"}},
    "nixpkgs-2511": {"locked": {"rev": "bbbbbbbb"}},
    "root": {"locked": {}}
  }
}`
	if err := os.WriteFile(filepath.Join(root, "flake.lock"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_SHA", "repo-commit")

	got, err := loadSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.ID, "sha256:") || len(got.ID) != 71 {
		t.Fatalf("snapshot ID=%q", got.ID)
	}
	if got.RepositoryCommit != "repo-commit" {
		t.Fatalf("repository commit=%q", got.RepositoryCommit)
	}
	if got.Channels["nixpkgs-unstable"] != "aaaaaaaa" || got.Channels["nixpkgs-2511"] != "bbbbbbbb" {
		t.Fatalf("channels=%v", got.Channels)
	}
}

func TestStoreHash(t *testing.T) {
	got, err := storeHash("/nix/store/c2h2f4cw9p8i8zcfy52fd1dd6g0yhnki-hello")
	if err != nil {
		t.Fatal(err)
	}
	if got != "c2h2f4cw9p8i8zcfy52fd1dd6g0yhnki" {
		t.Fatalf("hash=%q", got)
	}
}
