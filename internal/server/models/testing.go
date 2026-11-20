package models

// testing.go: the few hooks the tests of package server need to drive the downloads without the network. They are not
// used by the server.

import (
	"context"
	"net"
	"net/http"
)

// The states of a download, for the tests.
const (
	StateSucceeded = dlSucceeded
	StateFailed    = dlFailed
)

// AllowPrivateAddresses lets the downloads reach loopback and private addresses (the tests talk to local servers).
func (h *Handler) AllowPrivateAddresses() { h.dm.allowPrivate = true }

// SetKeys replaces where the Hugging Face and Civitai keys are read from; nil leaves one as it is.
func (h *Handler) SetKeys(hf, civitai func() string) {
	if hf != nil {
		h.dm.hfToken = hf
	}
	if civitai != nil {
		h.dm.civitaiToken = civitai
	}
}

// RouteAllTo makes every download connection reach addr, whatever host the URL names.
func (h *Handler) RouteAllTo(addr string) {
	tr := h.dm.client.Transport.(*http.Transport)
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
}

// Download is the record of a download by its identifier.
func (h *Handler) Download(id string) (Download, bool) {
	rec, _, ok := h.dm.get(id)
	return rec, ok
}

// HFAccessMessage is the guidance given when Hugging Face refuses a download.
func HFAccessMessage(status int, haveToken bool) string { return hfAccessMessage(status, haveToken) }

// CivitaiAccessMessage is the guidance given when Civitai refuses a download.
func CivitaiAccessMessage(status int, haveKey bool) string {
	return civitaiAccessMessage(status, haveKey)
}
