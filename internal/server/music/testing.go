package music

// testing.go: the few hooks the tests of package server need to drive the family without Whisper installed. They are
// not used by the server.

// SetWhisperDir points the Whisper models of this handler at another folder.
func (h *Handler) SetWhisperDir(dir string) { h.whisperDir = dir }

// SetDemucsDir points the weights of the voice separator of this handler at another folder.
func (h *Handler) SetDemucsDir(dir string) { h.demucsDir = dir }
