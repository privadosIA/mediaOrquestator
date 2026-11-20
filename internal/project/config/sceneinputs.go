package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SceneReferences are the files that guide an animated scene made with MiniMax H3's references bundle (Ref2VA), all
// of them in the project's sources folder and in the order the engine takes them. The scene's own picture (`image`) is
// reference picture 1, and Images are the others.
type SceneReferences struct {
	Images []string `yaml:"images,omitempty" json:"images,omitempty"`
	Videos []string `yaml:"videos,omitempty" json:"videos,omitempty"`
	Audios []string `yaml:"audios,omitempty" json:"audios,omitempty"`
}

// Empty reports whether there is no reference at all.
func (r *SceneReferences) Empty() bool {
	return r == nil || len(r.Images)+len(r.Videos)+len(r.Audios) == 0
}

// referenceAudioExts are the audio formats a reference audio can come in (ffmpeg brings it to what the engine reads).
var referenceAudioExts = map[string]bool{".mp3": true, ".wav": true, ".m4a": true, ".aac": true, ".ogg": true, ".flac": true}

// ValidReferenceAudioName reports whether name can be a reference audio: a plain audio file name.
func ValidReferenceAudioName(name string) bool {
	return sourceNamePattern.MatchString(name) && referenceAudioExts[strings.ToLower(filepath.Ext(name))]
}

// UsesFirstAndLastFrame says whether the engine reads the scene's picture as the first frame and takes a last one
// (MiniMax H3 with its first-frame bundle, and LTX 2.3).
func (a Animate) UsesFirstAndLastFrame() bool {
	return (a.IsMiniMaxH3() && a.FirstFrame) || a.IsLTX()
}

// UsesReferences says whether the engine takes references (MiniMax H3 with its references bundle).
func (a Animate) UsesReferences() bool { return a.IsMiniMaxH3() && !a.FirstFrame }

// validateSceneInputs checks the last picture and the references of an animated scene: the engine that takes them,
// the scene picture they hang from, the mixes that cannot be, and that every file exists with the right type.
func (c *Config) validateSceneInputs(s Scene, statPath func(string) string) []error {
	refs := s.References
	if s.EndImage == "" && refs.Empty() {
		return nil
	}
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("config: scene %q: "+format, append([]any{s.ID}, args...)...))
	}
	exists := func(field, name string, ok func(string) bool, kind string) {
		switch {
		case !ok(name):
			bad("%s %q must be the name of %s, not a path", field, name, kind)
		default:
			if _, err := os.Stat(statPath(filepath.Join(SourcesSubdir, name))); err != nil {
				bad("%s %q not found in %s", field, name, SourcesSubdir)
			}
		}
	}
	if !s.Animate {
		bad("end_image and references only work in an animated scene (animate: true)")
	}
	if s.Image == "" {
		bad("end_image and references need the scene's own picture (image): it is the first frame, or reference picture 1")
	}
	if s.EndImage != "" && !refs.Empty() {
		bad("end_image and references cannot be combined: they are different bundles of the engine")
	}
	if a := c.Defaults.Animate; a != nil && s.Animate {
		if s.EndImage != "" && !a.UsesFirstAndLastFrame() {
			bad("end_image needs minimax-h3 with first_frame or ltx-2.3 (engine is %q)", a.Engine)
		}
		if !refs.Empty() && !a.UsesReferences() {
			bad("references need minimax-h3 without first_frame (engine is %q)", a.Engine)
		}
	}
	if s.EndImage != "" {
		exists("end_image", s.EndImage, ValidImageName, "a .png, .jpg, .jpeg or .webp file")
	}
	if refs != nil {
		for _, n := range refs.Images {
			exists("references.images", n, ValidImageName, "a .png, .jpg, .jpeg or .webp file")
		}
		for _, n := range refs.Videos {
			exists("references.videos", n, ValidClipName, "an .mp4, .mov, .webm or .mkv file")
		}
		for _, n := range refs.Audios {
			exists("references.audios", n, ValidReferenceAudioName, "an .mp3, .wav, .m4a, .aac, .ogg or .flac file")
		}
	}
	return errs
}
