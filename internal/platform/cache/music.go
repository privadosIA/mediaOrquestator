package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Music mode phases. They are not in AllPhases: a Decision for a video scene
// never carries them.
const (
	PhaseCompose Phase = "compose"
	PhaseSynth   Phase = "synth"
)

// ComposeParams are the exact inputs that determine what the language model
// writes: the composed caption, the lyrics and metadata handed to it, the
// effective seed, the sampling parameters and the model file. Synthesis
// parameters are deliberately absent, so changing only those never re-runs
// the language model.
type ComposeParams struct {
	Caption          string
	Lyrics           string
	Instrumental     bool
	BPM              int
	Duration         float64
	KeyScale         string
	TimeSignature    string
	VocalLanguage    string
	Seed             int64
	LMTemperature    string
	LMCfgScale       string
	LMTopP           string
	LMTopK           string
	LMNegativePrompt string
	LMModel          string
}

// ComposeHash returns a deterministic SHA-256 hex digest over the compose
// inputs.
func ComposeHash(p ComposeParams) string {
	h := sha256.New()
	fmt.Fprintf(h, "caption=%s\x00lyrics=%s\x00instrumental=%t\x00", p.Caption, p.Lyrics, p.Instrumental)
	fmt.Fprintf(h, "bpm=%d\x00duration=%v\x00keyscale=%s\x00timesignature=%s\x00", p.BPM, p.Duration, p.KeyScale, p.TimeSignature)
	fmt.Fprintf(h, "vocal_language=%s\x00seed=%d\x00", p.VocalLanguage, p.Seed)
	fmt.Fprintf(h, "lm_temperature=%s\x00lm_cfg_scale=%s\x00lm_top_p=%s\x00lm_top_k=%s\x00", p.LMTemperature, p.LMCfgScale, p.LMTopP, p.LMTopK)
	fmt.Fprintf(h, "lm_negative_prompt=%s\x00lm_model=%s\x00", p.LMNegativePrompt, p.LMModel)
	return hex.EncodeToString(h.Sum(nil))
}

// SynthParams are the exact inputs that determine the audio: the content of
// the request handed to the synthesis program (which carries the composed
// lyrics and audio codes when the language model ran, and the synthesis
// parameters: steps, guidance, shift, solver, format, bitrate) and the
// model files.
type SynthParams struct {
	Request    []byte
	SynthModel string
	VAE        string
	// AdapterBytes is the size of the adapter's files (0 without adapter), so
	// replacing them under the same name re-synthesizes. Hashed only when set,
	// which keeps the hash of a song without adapter exactly as it was.
	AdapterBytes int64
	// SourceSHA is the SHA-256 of the song a cover, repaint or extract
	// transforms ("" for a new song, which keeps its hash as it was).
	SourceSHA string
	// RefSHA is the SHA-256 of the timbre reference ("" without one).
	RefSHA string
}

// SynthHash returns a deterministic SHA-256 hex digest over the synth inputs.
func SynthHash(p SynthParams) string {
	h := sha256.New()
	fmt.Fprintf(h, "synth_model=%s\x00vae=%s\x00", p.SynthModel, p.VAE)
	if p.AdapterBytes > 0 {
		fmt.Fprintf(h, "adapter_bytes=%d\x00", p.AdapterBytes)
	}
	if p.SourceSHA != "" {
		fmt.Fprintf(h, "source_sha=%s\x00", p.SourceSHA)
	}
	if p.RefSHA != "" {
		fmt.Fprintf(h, "ref_sha=%s\x00", p.RefSHA)
	}
	fmt.Fprint(h, "request=")
	h.Write(p.Request)
	return hex.EncodeToString(h.Sum(nil))
}

// FileSHA256 returns the hex SHA-256 of a file's content.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// MusicPaths returns the stable per-variant artifacts under tmp/: the
// enriched request written by the compose phase and the audio the synth
// phase produced. key is VariantKey(trackID, variants, i); ext is the audio
// extension (mp3 or wav).
func MusicPaths(key, ext string) (compose, audio string) {
	base := filepath.Join("tmp", fmt.Sprintf("track_%s", key))
	return base + ".compose.json", base + "." + ext
}

// MusicWorkDir is the scratch directory the engine writes into for one
// variant (it names its outputs after the input file, next to it).
func MusicWorkDir(key string) string {
	return filepath.Join("tmp", fmt.Sprintf("track_%s.work", key))
}

// FindCompose returns the enriched request of another cached variant made with
// exactly the same compose inputs, when its file is still on disk: the same
// inputs give the same request, so it can be reused under a new key (a track
// that went from three variants to one keeps the variant it chose).
func FindCompose(m *Manifest, base, hash string) (string, bool) {
	for _, id := range m.SortedSceneIDs() {
		e := m.Scenes[id]
		if e.ComposeHash == hash && e.ComposePath != "" {
			if _, err := os.Stat(filepath.Join(base, e.ComposePath)); err == nil {
				return e.ComposePath, true
			}
		}
	}
	return "", false
}

// FindSynth is FindCompose for the audio: another cached variant made from the
// same request and models.
func FindSynth(m *Manifest, base, hash string) (string, bool) {
	for _, id := range m.SortedSceneIDs() {
		e := m.Scenes[id]
		if e.SynthHash == hash && e.AudioPath != "" {
			if _, err := os.Stat(filepath.Join(base, e.AudioPath)); err == nil {
				return e.AudioPath, true
			}
		}
	}
	return "", false
}

// DecideCompose reports whether the compose phase of one variant is a cache
// hit: the stored hash matches and the enriched request is still on disk.
// base is the project directory the stored relative paths live in.
func DecideCompose(m *Manifest, base, key, newHash string, force bool) bool {
	if force {
		return false
	}
	e, ok := m.Scenes[key]
	if !ok || e.ComposeHash != newHash || e.ComposePath == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(base, e.ComposePath))
	return err == nil
}

// DecideSynth reports whether the synth phase of one variant is a cache hit:
// the stored hash matches and the audio file is still on disk.
func DecideSynth(m *Manifest, base, key, newHash string, force bool) bool {
	if force {
		return false
	}
	e, ok := m.Scenes[key]
	if !ok || e.SynthHash != newHash || e.AudioPath == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(base, e.AudioPath))
	return err == nil
}
