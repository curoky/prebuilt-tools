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

func remoteDigest(packageName, arch string) (string, error) {
	layer, err := remoteLayer(packageName, arch)
	if err != nil {
		return "", err
	}
	digest, err := layer.Digest()
	if err != nil {
		return "", fmt.Errorf("%s: %w", packageName, err)
	}
	return digest.String(), nil
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
