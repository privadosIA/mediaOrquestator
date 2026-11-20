// Package assemble turns a scene's rendered image and synthesized audio
// into a timed video clip via ffmpeg, and concatenates all scene clips
// into the final output video.
package assemble

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// CommandRunner
// ---------------------------------------------------------------------------

// CommandRunner abstracts ffmpeg subprocess execution so tests can inject a
// fake. args never includes the binary name itself.
type CommandRunner interface {
	Run(ctx context.Context, args []string) (stdout string, stderr string, err error)
}

// ---------------------------------------------------------------------------
// Scene / CacheManifest
// ---------------------------------------------------------------------------

// Scene describes one scene to be assembled into a clip.
//
// The camera-motion fields all default to "no effect": their zero values
// produce a clip byte-identical to the pre-camera-motion implementation.
type Scene struct {
	ID        string
	ImagePath string
	AudioPath string
	Duration  float64 // probed audio duration, or silence_seconds for silent scenes

	MotionType      string  // "" or "static" = frozen still (legacy behaviour)
	MotionIntensity float64 // [0,1]; scales movement amplitude
	Grain           float64 // [0,1]; 0 = no noise filter
	WhiteBalance    string  // "" or "neutral" = no adjustment filter
	Seed            int64   // phase source for deterministic handheld jitter
	Silent          bool    // replace AudioPath with a generated silent track
	Width           int     // configured scene resolution (zoompan/handheld output size)
	Height          int
	// SourceWidth and SourceHeight are the size of ImagePath when the scene
	// rendered at its own resolution. When set and different from Width x
	// Height, the still is first scaled to cover that size and cropped to it.
	SourceWidth  int
	SourceHeight int

	// AnimatedPath, when set, is the ping-pong MP4 from the animate phase;
	// it replaces the looped still, and AnimatedDuration is its length in
	// seconds. Motion presets do not apply to it.
	AnimatedPath     string
	AnimatedDuration float64
	// AnimatedSpeed, when > 0, is how fast the video plays (1 = its own speed); the
	// automatic slowdown that fits it to a longer scene is then not applied.
	AnimatedSpeed float64
	// AnimatedHold plays the animated clip once and holds its last frame for the rest of the scene, instead of looping
	// it: a clip that ends on a picture of the user's must not jump back to its start.
	AnimatedHold bool

	// OverlayPath, when set, is a full-frame video with an alpha channel (the rendered overlay
	// page, .webm VP9 or .mov ProRes 4444) drawn over the scene between OverlayStart and
	// OverlayEnd seconds; the overlay video starts at OverlayStart.
	OverlayPath  string
	OverlayStart float64
	OverlayEnd   float64

	// AmbientPath, when set, is the audio the animation model wrote (a WAV from the animate phase): it is mixed
	// under the scene's audio at AmbientGainDB, ducked by the narration, and plays once from the scene's start.
	AmbientPath string
	// AmbientSpeech says that audio carries a voice: it is mixed at full level (AmbientSpeechGainDB), not as ambience.
	AmbientSpeech bool
}

// AmbientGainDB is the fixed level of the model's ambience under the narration, set from the measured levels of
// the model's audio (mean -44 to -27 dB).
const AmbientGainDB = -18

// AmbientSpeechGainDB is the level of the model's audio when it carries a voice: the model writes it near full scale
// (mean -20 dB) and the voice has to be heard.
const AmbientSpeechGainDB = 0

// MaxAnimatedSlowdown caps how much an animated clip is stretched before it
// is looped to fill a longer scene.
const MaxAnimatedSlowdown = 2.0

// AnimatedSlowdown returns the setpts factor for fitting an animated clip of
// length source to a scene of length duration: min(2, duration/source) when
// the scene is longer, 1 (no speed change) otherwise.
func AnimatedSlowdown(duration, source float64) float64 {
	if source <= 0 || duration <= source {
		return 1
	}
	return math.Min(MaxAnimatedSlowdown, duration/source)
}

// animatedFactor is the setpts factor of a scene's video: the user's speed when set, the automatic fit otherwise.
func animatedFactor(scene Scene) float64 {
	if scene.AnimatedSpeed > 0 {
		return 1 / scene.AnimatedSpeed
	}
	return AnimatedSlowdown(scene.Duration, scene.AnimatedDuration)
}

// motionType resolves the scene's effective motion type.
func (s Scene) motionType() string {
	if s.MotionType == "" {
		return "static"
	}
	return s.MotionType
}

// CacheManifest tracks which artifacts are still referenced by the cache.
type CacheManifest struct {
	Artifacts []string `json:"artifacts"`
}

// ---------------------------------------------------------------------------
// ffmpeg argument builders (pure functions, easy to unit-test)
// ---------------------------------------------------------------------------

// SilentAudioSource is the lavfi source used in place of a WAV input for
// silent scenes: same sample rate and channel layout as narrated clips.
const SilentAudioSource = "anullsrc=r=44100:cl=stereo"

// BuildClipArgs returns the ffmpeg argument slice for encoding a single
// scene clip from a still image + audio track.
//
// Key invariants:
//   - image is looped with an explicit -framerate
//   - duration is controlled by -t (probed narration, or silence_seconds)
//   - video: libx264, -pix_fmt yuv420p; -tune stillimage only when the
//     image really is frozen (static motion)
//   - audio: aac 192k 44100 stereo — identical for narrated and silent clips
//   - every clip uses identical stream parameters so -c copy concat is valid
//   - a scene with zero-valued motion/look/silent fields produces exactly
//     the pre-camera-motion argument list
func BuildClipArgs(scene Scene, framerate int, outputPath string) []string {
	if framerate <= 0 {
		framerate = 30
	}
	args := []string{"-y"}
	if scene.AnimatedPath != "" && scene.AnimatedHold {
		// video input: the animated clip, once (the filter holds its last frame)
		args = append(args, "-i", scene.AnimatedPath)
	} else if scene.AnimatedPath != "" {
		// video input: the animated ping-pong clip, looped as long as needed
		args = append(args, "-stream_loop", "-1", "-i", scene.AnimatedPath)
	} else {
		// video input: loop the still image at an explicit framerate
		args = append(args, "-loop", "1", "-framerate", fmt.Sprintf("%d", framerate), "-i", scene.ImagePath)
	}
	// audio input: the synthesized WAV, or a generated silent track
	if scene.Silent {
		args = append(args, "-f", "lavfi", "-i", SilentAudioSource)
	} else {
		args = append(args, "-i", scene.AudioPath)
	}
	if scene.OverlayPath != "" {
		// The overlay is the third input. A VP9 file keeps its alpha only through the libvpx decoder.
		if strings.HasSuffix(scene.OverlayPath, ".webm") {
			args = append(args, "-c:v", "libvpx-vp9")
		}
		args = append(args, "-i", scene.OverlayPath)
	}
	// The ambience is the last input, so the narration (1) and the overlay (2) keep their indices.
	ambientInput := 2
	if scene.OverlayPath != "" {
		ambientInput = 3
	}
	if scene.AmbientPath != "" {
		args = append(args, "-i", scene.AmbientPath)
	}
	// duration: probed narration (or silence_seconds), not -shortest
	args = append(args, "-t", fmt.Sprintf("%.6f", scene.Duration))
	switch {
	case scene.OverlayPath != "" && scene.AmbientPath != "":
		args = append(args, "-filter_complex", overlayGraph(scene, framerate)+";"+ambientGraph(scene, ambientInput), "-map", "[v]", "-map", "[a]")
	case scene.OverlayPath != "":
		args = append(args, "-filter_complex", overlayGraph(scene, framerate), "-map", "[v]", "-map", "1:a")
	case scene.AmbientPath != "":
		if vf := BuildVideoFilter(scene, framerate); vf != "" {
			args = append(args, "-vf", vf)
		}
		args = append(args, "-filter_complex", ambientGraph(scene, ambientInput), "-map", "0:v", "-map", "[a]")
	default:
		if vf := BuildVideoFilter(scene, framerate); vf != "" {
			args = append(args, "-vf", vf)
		}
	}
	// video codec / tuning
	args = append(args, "-c:v", "libx264")
	if scene.motionType() == "static" && scene.AnimatedPath == "" {
		args = append(args, "-tune", "stillimage")
	}
	args = append(args,
		"-pix_fmt", "yuv420p",
		// audio codec: identical parameters for every clip
		"-c:a", "aac",
		"-b:a", "192k",
		"-ar", "44100",
		"-ac", "2",
		// output
		outputPath,
	)
	return args
}

// ambientGraph is the filter graph that mixes the ambience (input n) into the scene's audio (input 1) as the
// stream "a": lowered to AmbientGainDB and, when the scene has narration, compressed by it with the music's ducking
// constants. The ambience plays once; when it ends the scene's own audio goes on alone.
func ambientGraph(scene Scene, n int) string {
	gain := AmbientGainDB
	if scene.AmbientSpeech {
		gain = AmbientSpeechGainDB
	}
	amb := fmt.Sprintf("[%d:a]aresample=44100,aformat=channel_layouts=stereo,volume=%ddB", n, gain)
	narr := "[1:a]aresample=44100,aformat=channel_layouts=stereo"
	const mix = "amix=inputs=2:duration=first:normalize=0:dropout_transition=0"
	if scene.Silent {
		return amb + "[amb];" + narr + "[narr];[narr][amb]" + mix + "[a]"
	}
	return narr + ",asplit=2[narr][side];" + amb + "[amb];" +
		fmt.Sprintf("[amb][side]sidechaincompress=threshold=%s:ratio=%d:attack=%d:release=%d[duck];", num(duckThreshold), duckRatio, duckAttackMS, duckReleaseMS) +
		"[narr][duck]" + mix + "[a]"
}

// overlayGraph is the filter graph that draws the overlay (input 2) over the scene's own video
// chain (input 0): the overlay is shifted to start at OverlayStart and shown only until
// OverlayEnd, and the picture below stays visible around it.
func overlayGraph(scene Scene, framerate int) string {
	chain := BuildVideoFilter(scene, framerate)
	if chain == "" {
		chain = "null"
	}
	return fmt.Sprintf("[0:v]%s[base];[2:v]format=rgba,setpts=PTS-STARTPTS+%.6f/TB[ov];"+
		"[base][ov]overlay=0:0:format=auto:eof_action=pass:enable='between(t,%.6f,%.6f)'[v]",
		chain, scene.OverlayStart, scene.OverlayStart, scene.OverlayEnd)
}

// BuildVideoFilter composes the -vf chain for a scene: motion first, then
// white balance, then grain (grain last so it is not resampled by the
// zoom). An empty string means no filter flag at all.
func BuildVideoFilter(scene Scene, framerate int) string {
	var parts []string
	switch {
	case scene.AnimatedPath != "":
		// The video already moves: slow it to fit the scene, convert to the
		// project framerate and cover-crop to the project resolution. The
		// motion preset is deliberately not applied.
		parts = append(parts, fmt.Sprintf("setpts=%.6f*PTS", animatedFactor(scene)))
		if scene.AnimatedHold {
			parts = append(parts, fmt.Sprintf("tpad=stop_mode=clone:stop_duration=%.6f", scene.Duration))
		}
		parts = append(parts, fmt.Sprintf("fps=%d", framerate))
		if scene.Width > 0 && scene.Height > 0 {
			parts = append(parts,
				fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase", scene.Width, scene.Height),
				fmt.Sprintf("crop=%d:%d", scene.Width, scene.Height))
		}
		// The animation comes from a JPEG-range file (yuvj420p, full range), which many players and
		// browsers will not play as H.264: bring it to the normal limited range.
		parts = append(parts, "scale=out_range=tv")
	}
	if scene.AnimatedPath == "" {
		// A picture of another size (the scene rendered at its own resolution)
		// is brought to the project's size, cropped and never stretched.
		if scene.Width > 0 && scene.Height > 0 && scene.SourceWidth > 0 && scene.SourceHeight > 0 &&
			(scene.SourceWidth != scene.Width || scene.SourceHeight != scene.Height) {
			parts = append(parts,
				fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase", scene.Width, scene.Height),
				fmt.Sprintf("crop=%d:%d", scene.Width, scene.Height))
		}
		switch scene.motionType() {
		case "static":
			// no motion filter
		case "handheld":
			parts = append(parts, handheldFilter(scene))
		case "rotate_cw", "rotate_ccw":
			parts = append(parts, rotateFilter(scene))
		default:
			parts = append(parts, zoompanFilter(scene, framerate))
		}
	}
	switch scene.WhiteBalance {
	case "", "neutral":
	case "warm":
		parts = append(parts, "colortemperature=temperature=4600")
	case "cool":
		parts = append(parts, "colortemperature=temperature=8500")
	case "fluorescent":
		parts = append(parts, "colortemperature=temperature=4200,colorbalance=gm=.08")
	}
	if scene.Grain > 0 {
		strength := int(math.Round(20 * scene.Grain))
		if strength < 1 {
			strength = 1
		}
		parts = append(parts, fmt.Sprintf("noise=alls=%d:allf=t+u", strength))
	}
	return strings.Join(parts, ",")
}

// zoompanFilter builds the zoom/pan movement: a 2x lanczos pre-upscale (the
// standard mitigation for zoompan's integer-truncation stutter) feeding a
// zoompan dimensioned over the clip's full frame count, with linear —
// therefore continuous and jump-free — expressions.
func zoompanFilter(scene Scene, framerate int) string {
	frames := int(math.Round(scene.Duration * float64(framerate)))
	if frames < 2 {
		frames = 2
	}
	last := frames - 1

	var z, x, y string
	switch scene.motionType() {
	case "zoom_in":
		travel := 0.08 + 0.27*scene.MotionIntensity
		z = fmt.Sprintf("1+%.4f*on/%d", travel, last)
		x = "(iw-iw/zoom)/2"
		y = "(ih-ih/zoom)/2"
	case "zoom_out":
		travel := 0.08 + 0.27*scene.MotionIntensity
		z = fmt.Sprintf("%.4f-%.4f*on/%d", 1+travel, travel, last)
		x = "(iw-iw/zoom)/2"
		y = "(ih-ih/zoom)/2"
	case "pan_left":
		z = fmt.Sprintf("%.4f", 1.10+0.10*scene.MotionIntensity)
		x = fmt.Sprintf("(iw-iw/zoom)*(1-on/%d)", last)
		y = "(ih-ih/zoom)/2"
	case "pan_right":
		z = fmt.Sprintf("%.4f", 1.10+0.10*scene.MotionIntensity)
		x = fmt.Sprintf("(iw-iw/zoom)*on/%d", last)
		y = "(ih-ih/zoom)/2"
	}
	return fmt.Sprintf(
		"scale=iw*2:ih*2:flags=lanczos,zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d",
		z, x, y, frames, scene.Width, scene.Height, framerate,
	)
}

// RotationDegrees is how far a rotate preset has turned by the end of the
// clip: from 5 degrees (intensity 0) to 45 (intensity 1).
func RotationDegrees(intensity float64) float64 { return 5 + 40*intensity }

// rotateFilter builds the rotation: the frame is zoomed once by the factor the
// largest angle needs to leave no empty corner (cos θ + sin θ · the larger
// aspect ratio), rotated about its center by an angle that grows linearly with
// time to θ at the end of the clip (negative for counter-clockwise), and cropped
// back to the configured resolution.
func rotateFilter(scene Scene) string {
	w, h := scene.Width, scene.Height
	if w <= 0 || h <= 0 {
		w, h = 16, 9 // only the proportion matters for the zoom
	}
	theta := RotationDegrees(scene.MotionIntensity) * math.Pi / 180
	aspect := math.Max(float64(w)/float64(h), float64(h)/float64(w))
	zoom := math.Cos(theta) + math.Sin(theta)*aspect
	sign := ""
	if scene.motionType() == "rotate_ccw" {
		sign = "-"
	}
	d := scene.Duration
	if d <= 0 {
		d = 1
	}
	return fmt.Sprintf(
		"scale=iw*%.4f:ih*%.4f:flags=lanczos,rotate=a='%s%.5f*t/%.4f':ow=iw:oh=ih:c=black:bilinear=1,crop=%d:%d",
		zoom, zoom, sign, theta, d, scene.Width, scene.Height,
	)
}

// handheldFilter builds the camera-shake movement: a crop whose offsets sum
// two low-frequency sinusoids (an organic drift without randomness), with
// phases derived deterministically from the scene's seed, scaled back to
// the configured resolution. Identical (scene, seed) inputs yield
// byte-identical filter arguments, keeping the clip cacheable.
func handheldFilter(scene Scene) string {
	margin := 2 * int(math.Ceil(4+12*scene.MotionIntensity)) // 8..32 px, even
	amplitude := float64(margin)/2 - 1

	const phi = 1.6180339887
	phase := func(k float64) float64 {
		v := float64(scene.Seed) * k / phi
		return 2 * math.Pi * (v - math.Floor(v))
	}
	p1, p2, p3 := phase(1), phase(2), phase(3)

	return fmt.Sprintf(
		"crop=w=iw-%d:h=ih-%d:"+
			"x='(iw-ow)/2+%.2f*(0.6*sin(2*PI*1.1*t+%.4f)+0.4*sin(2*PI*2.3*t+%.4f))':"+
			"y='(ih-oh)/2+%.2f*(0.6*cos(2*PI*0.9*t+%.4f)+0.4*cos(2*PI*1.7*t+%.4f))',"+
			"scale=%d:%d",
		margin, margin,
		amplitude, p1, p2,
		amplitude, p1, p3,
		scene.Width, scene.Height,
	)
}

// BuildFitImageArgs returns the ffmpeg arguments that turn a given picture into
// a scene image: scaled to cover width x height, cropped to exactly that size,
// one frame.
func BuildFitImageArgs(input string, width, height int, output string) []string {
	return []string{
		"-y",
		"-i", input,
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", width, height, width, height),
		"-frames:v", "1",
		output,
	}
}

// BuildPosterArgs returns the ffmpeg arguments that take the first frame of a
// video as a scene image, scaled to cover width x height and cropped to
// exactly that size.
func BuildPosterArgs(input string, width, height int, start float64, output string) []string {
	args := BuildFitImageArgs(input, width, height, output)
	if start > 0 {
		args = append([]string{"-y", "-ss", fmt.Sprintf("%.3f", start)}, args[1:]...)
	}
	return args
}

// BuildCutArgs returns the ffmpeg arguments that cut length seconds of a video
// from start into output, re-encoded (video only) so the cut is exact on any frame.
func BuildCutArgs(input string, start, length float64, output string) []string {
	return []string{
		"-y",
		"-ss", fmt.Sprintf("%.3f", start),
		"-t", fmt.Sprintf("%.3f", length),
		"-i", input,
		"-an",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		output,
	}
}

// BuildConcatListContent returns the text content of a concat-demuxer list
// file referencing the given clip paths (absolute or relative to the list
// file's directory).
func BuildConcatListContent(clips []string) string {
	var b strings.Builder
	for _, c := range clips {
		// The concat demuxer expects: file '<path>'
		// Escape single quotes in the path.
		escaped := strings.ReplaceAll(c, "'", `'\''`)
		fmt.Fprintf(&b, "file '%s'\n", escaped)
	}
	return b.String()
}

// BuildConcatArgs returns the ffmpeg argument slice for concatenating clips
// using the concat demuxer, encoding the result once.
func BuildConcatArgs(listFile, outputPath string) []string {
	return []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		// The result is encoded once, picture and sound. The clips are encoded one by one and a stream
		// made by copying them is not one the browsers agree on: each AAC stream starts with its own
		// encoder delay, which leaves overlapping audio timestamps at every join (Safari and QuickTime
		// stall), and Firefox does not decode the copied picture at all.
		"-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "192k", "-ar", "44100", "-ac", "2",
		"-movflags", "+faststart",
		outputPath,
	}
}

// JoinClip is one scene clip of the final join.
type JoinClip struct {
	Path     string
	Duration float64 // seconds
	// Fade is the length of the crossfade into this clip from the previous one; 0 is a cut.
	// The first clip's is ignored.
	Fade float64
	// Kind is the xfade transition into this clip ("" is "fade", the crossfade).
	Kind string
	// ShaderVideo is, for a join drawn by a shader transition, the video of the transition (Fade seconds, no sound, the size and frame rate
	// of the clips): it takes the place of the xfade picture, the sound still crossfades. Empty for any other join.
	ShaderVideo string
	// Accent is the glow colour of a shader transition into this clip ("#rrggbb"); empty is the shader's own.
	Accent string
}

// HasFades reports whether any join between two clips is a crossfade.
func HasFades(clips []JoinClip) bool {
	for i, c := range clips {
		if i > 0 && c.Fade > 0 {
			return true
		}
	}
	return false
}

// BuildCrossfadeArgs returns the ffmpeg arguments that join clips with crossfades
// where Fade > 0 and cuts elsewhere, in one call that encodes the result once. The
// clips are split into runs separated by cuts; each run is chained with xfade
// (picture) and acrossfade (sound), whose offset is the length of the run so far
// minus the fade, and the runs are joined with the concat filter. Every clip has the
// same size, frame rate and audio format, which xfade requires.
func BuildCrossfadeArgs(clips []JoinClip, framerate int, outputPath string) []string {
	if framerate <= 0 {
		framerate = 30
	}
	args := []string{"-y"}
	for _, c := range clips {
		args = append(args, "-i", c.Path)
	}
	// the videos of the shader transitions are inputs after the clips, in the order of their joins
	shaderInput := map[int]int{}
	for i, c := range clips {
		if i > 0 && c.Fade > 0 && c.ShaderVideo != "" {
			shaderInput[i] = len(clips) + len(shaderInput)
			args = append(args, "-i", c.ShaderVideo)
		}
	}
	var filters, tails []string
	// run holds the labels of the picture and sound of the current run, and its length so far.
	var vcur, acur string
	var length float64
	group := 0
	flush := func() {
		if vcur == "" {
			return
		}
		tails = append(tails, vcur, acur)
		group++
	}
	for i, c := range clips {
		if i == 0 || c.Fade <= 0 {
			flush()
			vcur, acur, length = fmt.Sprintf("[%d:v]", i), fmt.Sprintf("[%d:a]", i), c.Duration
			continue
		}
		v, a := fmt.Sprintf("[v%d]", i), fmt.Sprintf("[a%d]", i)
		kind := c.Kind
		if kind == "" {
			kind = "fade"
		}
		if si, ok := shaderInput[i]; ok {
			// the run without its last Fade seconds, the transition, and the clip without its first Fade seconds
			filters = append(filters,
				fmt.Sprintf("%strim=end=%.3f,setpts=PTS-STARTPTS[h%d]", vcur, length-c.Fade, i),
				fmt.Sprintf("[%d:v]trim=start=%.3f,setpts=PTS-STARTPTS[b%d]", i, c.Fade, i),
				fmt.Sprintf("[h%d][%d:v][b%d]concat=n=3:v=1:a=0%s", i, si, i, v))
		} else {
			filters = append(filters, fmt.Sprintf("%s[%d:v]xfade=transition=%s:duration=%.3f:offset=%.3f%s", vcur, i, kind, c.Fade, length-c.Fade, v))
		}
		filters = append(filters, fmt.Sprintf("%s[%d:a]acrossfade=d=%.3f%s", acur, i, c.Fade, a))
		vcur, acur = v, a
		length += c.Duration - c.Fade
	}
	flush()
	filters = append(filters, fmt.Sprintf("%sconcat=n=%d:v=1:a=1[vc][aout]", strings.Join(tails, ""), group))
	filters = append(filters, fmt.Sprintf("[vc]fps=%d,format=yuv420p[vout]", framerate))
	args = append(args,
		"-filter_complex", strings.Join(filters, ";"),
		"-map", "[vout]", "-map", "[aout]",
		"-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "192k", "-ar", "44100", "-ac", "2",
		"-movflags", "+faststart",
		outputPath)
	return args
}

// JoinWithFades joins clips with BuildCrossfadeArgs.
func JoinWithFades(ctx context.Context, runner CommandRunner, clips []JoinClip, framerate int, outputPath string) (string, error) {
	_, stderr, err := runner.Run(ctx, BuildCrossfadeArgs(clips, framerate, outputPath))
	if err != nil {
		return stderr, fmt.Errorf("join with fades: %w", err)
	}
	return "", nil
}

// ---------------------------------------------------------------------------
// Pipeline operations
// ---------------------------------------------------------------------------

// GenerateClip runs ffmpeg to produce one scene clip.
func GenerateClip(ctx context.Context, runner CommandRunner, scene Scene, framerate int, outputPath string) (string, error) {
	args := BuildClipArgs(scene, framerate, outputPath)
	_, stderr, err := runner.Run(ctx, args)
	if err != nil {
		return stderr, fmt.Errorf("clip generation for scene %q: %w", scene.ID, err)
	}
	return "", nil
}

// ConcatClips runs ffmpeg concat-demuxer to join all clips.
func ConcatClips(ctx context.Context, runner CommandRunner, listFile, outputPath string) (string, error) {
	args := BuildConcatArgs(listFile, outputPath)
	_, stderr, err := runner.Run(ctx, args)
	if err != nil {
		return stderr, fmt.Errorf("concat: %w", err)
	}
	return "", nil
}

// WriteConcatList writes the concat-demuxer list file under tmpDir.
func WriteConcatList(tmpDir string, clips []string) (string, error) {
	content := BuildConcatListContent(clips)
	listPath := filepath.Join(tmpDir, "concat_list.txt")
	if err := os.WriteFile(listPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write concat list: %w", err)
	}
	return listPath, nil
}

// CleanupTmp removes the tmp directory unless keepTmp is true.
// Artifacts still referenced by the cache manifest are preserved.
func CleanupTmp(tmpDir string, manifest *CacheManifest, keepTmp bool) error {
	if keepTmp {
		return nil
	}
	if manifest == nil {
		return os.RemoveAll(tmpDir)
	}
	// Build set of referenced artifacts.
	referenced := make(map[string]struct{}, len(manifest.Artifacts))
	for _, a := range manifest.Artifacts {
		referenced[filepath.Clean(a)] = struct{}{}
	}
	// Walk tmpDir and remove files not in the referenced set.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		p := filepath.Join(tmpDir, e.Name())
		clean := filepath.Clean(p)
		if _, ok := referenced[clean]; ok {
			continue // keep: still referenced by cache manifest
		}
		if e.IsDir() {
			_ = os.RemoveAll(p)
		} else {
			_ = os.Remove(p)
		}
	}
	// Remove the directory itself if empty.
	remaining, _ := os.ReadDir(tmpDir)
	if len(remaining) == 0 {
		_ = os.Remove(tmpDir)
	}
	return nil
}
