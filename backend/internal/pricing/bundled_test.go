package pricing

import (
	"testing"
)

func TestBundledCatalogParsesAndPricesEveryProvider(t *testing.T) {
	catalog, err := BundledCatalog()
	if err != nil {
		t.Fatalf("BundledCatalog: %v", err)
	}
	snapshot := catalog.Snapshot()
	for _, provider := range []string{"anthropic", "openai", "zai"} {
		if snapshot.ProviderVersion(provider) == "" {
			t.Errorf("bundled catalog has no %s provider", provider)
		}
	}
}

func TestBundledFetcherServesCatalogOnlyWithoutCache(t *testing.T) {
	fetcher := BundledFetcher{}
	first, err := fetcher.Fetch(t.Context(), "", false)
	if err != nil {
		t.Fatalf("Fetch without cache: %v", err)
	}
	if first.Catalog == nil || first.NotModified {
		t.Fatalf("Fetch without cache = %+v, want the bundled catalog", first)
	}
	// With a cached catalog (possibly newer, from an earlier build that could
	// download), the bundled snapshot must never replace it.
	cached, err := fetcher.Fetch(t.Context(), "etag-1", true)
	if err != nil {
		t.Fatalf("Fetch with cache: %v", err)
	}
	if cached.Catalog != nil || !cached.NotModified || cached.ETag != "etag-1" {
		t.Fatalf("Fetch with cache = %+v, want not-modified preserving the validator", cached)
	}
}
