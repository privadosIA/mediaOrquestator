//go:build !private

package server

// privateCatalogJSON is empty in the public edition: it has no catalog entries
// that need a Civitai key.
var privateCatalogJSON []byte
