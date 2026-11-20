package music

import "mediaOrquestator/internal/platform/edition"

func init() { edition.AllowProviderHostsForTest("127.0.0.1", "example.com") }
