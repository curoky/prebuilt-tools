package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v75/github"
)

// GHCR does not implement the OCI `DELETE /v2/.../manifests/<digest>` endpoint
// (it returns 405 UNSUPPORTED). The only way to delete a package version is the
// GitHub Packages REST API, keyed by a numeric version id rather than an OCI
// tag. ghcrClient wraps go-github: it lists container package versions to build
// a tag -> version id map, then deletes by id.
type ghcrClient struct {
	api   *github.Client
	owner string
	pkg   string

	// isOrg records whether the package lives under an organization; resolved
	// lazily on the first listing so both user- and org-owned caches work.
	isOrg    bool
	resolved bool
}

const packageType = "container"

// newGHCRClient derives the owner and package name from a GHCR repository
// reference like "curoky/standalone-binaries-cache" and reads GITHUB_TOKEN for
// authentication. Deleting package versions requires a token with admin
// permission on the package (classic PAT needs `delete:packages`).
func newGHCRClient(repository string) (*ghcrClient, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is required to delete GHCR package versions")
	}
	owner, pkg, ok := strings.Cut(repository, "/")
	if !ok || owner == "" || pkg == "" {
		return nil, fmt.Errorf("cannot derive owner/package from repository %q", repository)
	}
	return &ghcrClient{
		api:   github.NewClient(nil).WithAuthToken(token),
		owner: owner,
		pkg:   pkg,
	}, nil
}

// versionsByTag lists every version of the container package and returns a map
// from OCI tag to version id. A version may carry several tags; each is mapped.
func (client *ghcrClient) versionsByTag(ctx context.Context) (map[string]int64, error) {
	byTag := make(map[string]int64)
	opts := &github.PackageListOptions{ListOptions: github.ListOptions{PerPage: 100}}
	for {
		versions, resp, err := client.listVersions(ctx, opts)
		if err != nil {
			return nil, err
		}
		for _, version := range versions {
			metadata, ok := version.GetMetadata()
			if !ok || metadata.Container == nil {
				continue
			}
			for _, tag := range metadata.Container.Tags {
				byTag[tag] = version.GetID()
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return byTag, nil
}

// listVersions fetches one page of package versions, resolving whether the
// package is user- or org-owned on the first call and remembering it.
func (client *ghcrClient) listVersions(ctx context.Context, opts *github.PackageListOptions) ([]*github.PackageVersion, *github.Response, error) {
	if client.resolved {
		if client.isOrg {
			return client.api.Organizations.PackageGetAllVersions(ctx, client.owner, packageType, client.pkg, opts)
		}
		return client.api.Users.PackageGetAllVersions(ctx, client.owner, packageType, client.pkg, opts)
	}
	versions, resp, err := client.api.Users.PackageGetAllVersions(ctx, client.owner, packageType, client.pkg, opts)
	if err == nil {
		client.isOrg = false
		client.resolved = true
		return versions, resp, nil
	}
	if resp == nil || resp.StatusCode != 404 {
		return nil, resp, err
	}
	versions, resp, orgErr := client.api.Organizations.PackageGetAllVersions(ctx, client.owner, packageType, client.pkg, opts)
	if orgErr != nil {
		return nil, resp, orgErr
	}
	client.isOrg = true
	client.resolved = true
	return versions, resp, nil
}

// deleteVersion deletes a single package version by id.
func (client *ghcrClient) deleteVersion(ctx context.Context, versionID int64) error {
	if client.isOrg {
		_, err := client.api.Organizations.PackageDeleteVersion(ctx, client.owner, packageType, client.pkg, versionID)
		return err
	}
	_, err := client.api.Users.PackageDeleteVersion(ctx, client.owner, packageType, client.pkg, versionID)
	return err
}
