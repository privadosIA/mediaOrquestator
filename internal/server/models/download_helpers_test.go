package models

import "testing"

func startDL(t *testing.T, s *testServer, body string) Download {
	t.Helper()
	_, d := postDownload(t, s, body)
	return waitDL(t, s, d.ID, dlSucceeded, dlFailed)
}
