package models

import (
	"errors"
	"strings"
)

// The states of a download as other families read them.
const (
	DownloadSucceeded = dlSucceeded
	DownloadFailed    = dlFailed
	DownloadCancelled = dlCancelled
)

// ErrAlreadyInstalled is what StartCatalog answers when every file of the entry is there.
var ErrAlreadyInstalled = errors.New("the entry is already installed")

// DownloadError is a download that could not start, with the HTTP status it would answer with.
type DownloadError struct {
	Status int
	Msg    string
}

func (e *DownloadError) Error() string { return e.Msg }

// StartCatalog starts the download of the missing files of a catalog entry (the same as POST /api/downloads with its id) and returns its record. The caller
// follows it with Download and stops it with CancelDownload. An entry that is complete answers ErrAlreadyInstalled.
func (h *Handler) StartCatalog(id string) (Download, error) {
	rec, derr := h.startDownload(downloadRequest{ID: id})
	if derr == nil {
		return rec, nil
	}
	if derr.status == 409 && strings.Contains(derr.msg, "already installed") {
		return Download{}, ErrAlreadyInstalled
	}
	return Download{}, &DownloadError{Status: derr.status, Msg: derr.msg}
}

// CancelDownload stops a download that is queued or running (one that ended is left as it is).
func (h *Handler) CancelDownload(id string) {
	_, _ = h.dm.cancelJob(id)
}
