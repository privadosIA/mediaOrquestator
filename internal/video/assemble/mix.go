package assemble

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MixOptions describes how background music goes under the narration.
type MixOptions struct {
	Volume  float64 // linear gain of the music before ducking
	Fade    float64 // seconds of fade in and out
	Ducking bool    // lower the music while the narration speaks
	// Duration is the length of the video in seconds: where the fade out ends
	// and how far the looped music is cut.
	Duration float64
}

// Ducking and loudness constants, kept together because they were set by ear.
const (
	duckThreshold = 0.03 // sidechain level (linear) above which the music is lowered
	duckRatio     = 8
	duckAttackMS  = 20
	duckReleaseMS = 500
	targetLUFS    = -16
	truePeakDB    = -1.5
	loudRange     = 11
)

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// MixFilter returns the filtergraph that turns the video's narration (input
// 0) and the music (input 1) into the single audio stream "a".
func MixFilter(o MixOptions) string {
	fade := o.Fade
	if fade*2 > o.Duration {
		fade = o.Duration / 2 // a short video cannot fade longer than half of itself
	}
	music := fmt.Sprintf("[1:a]aresample=44100,aformat=channel_layouts=stereo,volume=%s", num(o.Volume))
	if fade > 0 {
		music += fmt.Sprintf(",afade=t=in:st=0:d=%s,afade=t=out:st=%s:d=%s", num(fade), num(o.Duration-fade), num(fade))
	}
	var parts []string
	if o.Ducking {
		parts = append(parts,
			"[0:a]asplit=2[narr][side]",
			music+"[bg]",
			fmt.Sprintf("[bg][side]sidechaincompress=threshold=%s:ratio=%d:attack=%d:release=%d[duck]", num(duckThreshold), duckRatio, duckAttackMS, duckReleaseMS),
			"[narr][duck]amix=inputs=2:duration=first:normalize=0,"+loudnorm()+"[a]",
		)
	} else {
		parts = append(parts,
			music+"[bg]",
			"[0:a][bg]amix=inputs=2:duration=first:normalize=0,"+loudnorm()+"[a]",
		)
	}
	return strings.Join(parts, ";")
}

func loudnorm() string {
	return fmt.Sprintf("loudnorm=I=%d:TP=%s:LRA=%d", targetLUFS, num(truePeakDB), loudRange)
}

// BuildMixArgs returns the ffmpeg arguments that mix music under the
// narration of video into out. The video stream is copied; the audio is
// encoded like every clip (AAC 192k, 44100 Hz, stereo). The music is looped
// when it is shorter than the video.
func BuildMixArgs(video, music, out string, o MixOptions) []string {
	return []string{
		"-y",
		"-i", video,
		"-stream_loop", "-1", "-i", music,
		"-filter_complex", MixFilter(o),
		"-map", "0:v",
		"-map", "[a]",
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "192k",
		"-ar", "44100",
		"-ac", "2",
		"-movflags", "+faststart",
		out,
	}
}

// MixMusic runs the mix.
func MixMusic(ctx context.Context, runner CommandRunner, video, music, out string, o MixOptions) (string, error) {
	_, stderr, err := runner.Run(ctx, BuildMixArgs(video, music, out, o))
	if err != nil {
		return stderr, fmt.Errorf("mix: %w", err)
	}
	return "", nil
}
