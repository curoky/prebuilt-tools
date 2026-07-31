package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"golang.org/x/sync/errgroup"
)

// ociRegistry is the registry reference root. It is a variable so tests can
// point it at a local httptest registry; production always uses ghcr.
var ociRegistry = defaultRegistry

func ref(packageName, arch string) string {
	return fmt.Sprintf("%s:%s-%s", ociRegistry, packageName, arch)
}

func isNotFound(err error) bool {
	var transportError *transport.Error
	return errors.As(err, &transportError) && transportError.StatusCode == http.StatusNotFound
}

type artifactRequest struct {
	name string
	arch string
}

type packageArtifact struct {
	name   string
	arch   string
	layer  v1.Layer
	digest string
}

type artifactDownload struct {
	packageArtifact
	destination string
}

// remoteLayer returns the published content layer. Its digest is the package
// version and its compressed stream is the package tarball.
func remoteLayer(packageName, arch string) (v1.Layer, error) {
	if err := validatePackageName(packageName); err != nil {
		return nil, err
	}
	if err := validateArch(arch); err != nil {
		return nil, err
	}
	reference, err := name.ParseReference(ref(packageName, arch), name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", packageName, err)
	}
	image, err := remote.Image(reference)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%s: not found for arch %q", packageName, arch)
		}
		return nil, fmt.Errorf("%s: %w", packageName, err)
	}
	layers, err := image.Layers()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", packageName, err)
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("%s: image has no layers", packageName)
	}
	return layers[len(layers)-1], nil
}

func resolveArtifact(request artifactRequest) (packageArtifact, error) {
	layer, err := remoteLayer(request.name, request.arch)
	if err != nil {
		return packageArtifact{}, err
	}
	digest, err := layer.Digest()
	if err != nil {
		return packageArtifact{}, fmt.Errorf("%s: %w", request.name, err)
	}
	return packageArtifact{
		name:   request.name,
		arch:   request.arch,
		layer:  layer,
		digest: digest.String(),
	}, nil
}

func remoteDigest(packageName, arch string) (string, error) {
	artifact, err := resolveArtifact(artifactRequest{name: packageName, arch: arch})
	if err != nil {
		return "", err
	}
	return artifact.digest, nil
}

func resolveArtifacts(requests []artifactRequest) ([]packageArtifact, error) {
	artifacts := make([]packageArtifact, len(requests))
	resolveErrors := make([]error, len(requests))
	var group errgroup.Group
	group.SetLimit(maxParallel)
	for index := range requests {
		group.Go(func() error {
			request := requests[index]
			artifact, err := resolveArtifact(request)
			if err != nil {
				resolveErrors[index] = err
				return nil
			}
			artifacts[index] = artifact
			return nil
		})
	}
	_ = group.Wait()
	return artifacts, errors.Join(resolveErrors...)
}

func downloadArtifacts(downloads []artifactDownload) error {
	var group errgroup.Group
	group.SetLimit(maxParallel)
	for index := range downloads {
		group.Go(func() error {
			download := downloads[index]
			if err := downloadLayer(download.layer, download.destination); err != nil {
				return fmt.Errorf("%s: %w", download.name, err)
			}
			return nil
		})
	}
	return group.Wait()
}

// downloadLayer atomically replaces dst after the complete compressed layer
// has been written.
func downloadLayer(layer v1.Layer, dst string) error {
	reader, err := layer.Compressed()
	if err != nil {
		return err
	}
	defer reader.Close()
	return writeAtomic(dst, 0o600, reader)
}

func cachePath(arch, packageName string) string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(base, "binman", arch, packageName+".tar.gz")
}
