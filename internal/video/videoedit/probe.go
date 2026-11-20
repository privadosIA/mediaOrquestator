package videoedit

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Info is what ffprobe says of a video.
type Info struct {
	Width, Height int
	FPS           float64
	// FPSText is the rate as ffprobe wrote it ("30000/1001"), exact where FPS is not.
	FPSText  string
	Duration float64
	HasAudio bool
	// Comment is the comment tag of the file, where the tools leave what they want to remember (see Extended).
	Comment string
}

// ProbeFunc reads a video. The Catalog holds the one the tools and the server call, so a test can replace it.
type ProbeFunc func(ctx context.Context, path string) (Info, error)

func ffprobe(ctx context.Context, path string) (Info, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-print_format", "json", "-show_streams", "-show_format", path).Output()
	if err != nil {
		return Info{}, fmt.Errorf("ffprobe cannot read the file: %w", err)
	}
	return parseProbe(out)
}

func parseProbe(raw []byte) (Info, error) {
	var p struct {
		Streams []struct {
			CodecType  string `json:"codec_type"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
			AvgRate    string `json:"avg_frame_rate"`
			Duration   string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return Info{}, fmt.Errorf("ffprobe answered something that is not JSON: %w", err)
	}
	var info Info
	found := false
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			if found || s.Width <= 0 || s.Height <= 0 {
				continue
			}
			found = true
			info.Width, info.Height = s.Width, s.Height
			info.FPS, info.FPSText = rate(s.AvgRate), s.AvgRate
			if info.FPS == 0 {
				info.FPS, info.FPSText = rate(s.RFrameRate), s.RFrameRate
			}
			if d, err := strconv.ParseFloat(s.Duration, 64); err == nil {
				info.Duration = d
			}
		case "audio":
			info.HasAudio = true
		}
	}
	if !found {
		return Info{}, fmt.Errorf("the file has no video stream")
	}
	for k, v := range p.Format.Tags {
		if strings.EqualFold(k, "comment") {
			info.Comment = v
		}
	}
	if info.Duration == 0 {
		if d, err := strconv.ParseFloat(p.Format.Duration, 64); err == nil {
			info.Duration = d
		}
	}
	return info, nil
}

// rate reads "30000/1001" or "25".
func rate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	if !ok {
		return n
	}
	d, err := strconv.ParseFloat(den, 64)
	if err != nil || d == 0 {
		return 0
	}
	return n / d
}
