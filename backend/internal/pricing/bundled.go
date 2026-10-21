package pricing

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
)

// bundledCatalogFS is a snapshot of the reviewed catalog in the repository's
// pricing/catalog/v1, embedded so a local-only build can price usage without
// downloading anything. To refresh it, copy pricing/catalog/v1/manifest.json
// and the provider files that manifest references into bundled/v1.
//
//go:embed bundled/v1
var bundledCatalogFS embed.FS

// BundledFetcher is a CatalogFetcher that never touches the network. It serves
// the embedded catalog only when no cached catalog exists; once one is cached
// it reports not-modified, so a catalog fetched by an earlier build is never
// replaced by an older embedded snapshot.
type BundledFetcher struct{}

// Fetch implements CatalogFetcher.
func (BundledFetcher) Fetch(_ context.Context, etag string, cacheAvailable bool) (FetchResult, error) {
	if cacheAvailable {
		return FetchResult{ETag: etag, NotModified: true}, nil
	}
	catalog, err := BundledCatalog()
	if err != nil {
		return FetchResult{}, err
	}
	return FetchResult{Catalog: catalog}, nil
}

// BundledCatalog parses and validates the embedded catalog snapshot.
func BundledCatalog() (*Catalog, error) {
	root, err := fs.Sub(bundledCatalogFS, "bundled/v1")
	if err != nil {
		return nil, fmt.Errorf("open bundled pricing catalog: %w", err)
	}
	manifestBytes, err := fs.ReadFile(root, "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("read bundled pricing manifest: %w", err)
	}
	var manifest catalogManifest
	if err := decodeStrictJSON(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("decode bundled pricing manifest: %w", err)
	}
	providerBytes := make(map[string][]byte, len(manifest.Providers))
	for _, ref := range manifest.Providers {
		if !safeProviderPath(ref.Path) {
			return nil, fmt.Errorf("unsafe bundled provider path %q", ref.Path)
		}
		contents, err := fs.ReadFile(root, path.Clean(ref.Path))
		if err != nil {
			return nil, fmt.Errorf("read bundled provider %q: %w", ref.ProviderID, err)
		}
		providerBytes[ref.Path] = contents
	}
	catalog, err := ParseCatalog(manifestBytes, providerBytes)
	if err != nil {
		return nil, fmt.Errorf("validate bundled pricing catalog: %w", err)
	}
	return catalog, nil
}
