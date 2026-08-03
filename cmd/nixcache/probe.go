package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/nix-community/go-nix/pkg/narinfo"
)

const probeErrorExitCode = 2

func probeStorePath(ctx context.Context, client *http.Client, baseURL, storePath string) (bool, error) {
	hash, err := storeHash(storePath)
	if err != nil {
		return false, err
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return false, fmt.Errorf("parse cache URL: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, hash+narInfoSuffix)
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false, err
	}
	response, err := client.Do(request)
	if err != nil {
		return false, fmt.Errorf("query cache: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK:
		info, err := narinfo.Parse(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			return false, fmt.Errorf("parse narinfo for %s: %w", storePath, err)
		}
		if err := info.Check(); err != nil {
			return false, fmt.Errorf("validate narinfo for %s: %w", storePath, err)
		}
		if info.StorePath != storePath {
			return false, fmt.Errorf("cache returned StorePath %q for %q", info.StorePath, storePath)
		}
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return false, fmt.Errorf("query cache: %s: %s", response.Status, body)
	}
}
