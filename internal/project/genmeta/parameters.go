package genmeta

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

const (
	negMarker   = "Negative prompt:"
	sdcppMarker = "SDCPP:"
)

// paramRe finds the "Key: " starts of the "Steps: 20, Sampler: euler, …" line. A key is only
// recognized at the start of the line or after ", ", so a value that contains a comma
// (a model name) does not break the next key; unknown keys end the value before them.
var paramRe = regexp.MustCompile(`(?:^|, )([A-Z][A-Za-z0-9 ]{0,30}): `)

// fromParameters reads the A1111 / stable-diffusion.cpp text:
//
//	<prompt>
//	Negative prompt: <negative>
//	Steps: 25, Sampler: euler, CFG scale: 7.0, Seed: 42, Size: 512x512, Model: x[, SDCPP: {json}]
//
// Without the "Steps:" line there is no way to tell it from any other text, so
// it yields nothing.
func fromParameters(text string) Metadata {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	pi := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "Steps: ") {
			pi = i
			break
		}
	}
	if pi < 0 {
		return Metadata{}
	}
	var m Metadata
	head := strings.Join(lines[:pi], "\n")
	if i := strings.Index(head, negMarker); i >= 0 {
		// A prompt line can contain the marker only at the start of a line.
		if j := indexLineStart(head, negMarker); j >= 0 {
			m.Prompt = head[:j]
			m.NegativePrompt = strings.TrimSpace(head[j+len(negMarker):])
		} else {
			m.Prompt = head
		}
	} else {
		m.Prompt = head
	}

	params := lines[pi]
	var tail string
	if i := strings.Index(params, ", "+sdcppMarker+" "); i >= 0 {
		tail = params[i+len(", "+sdcppMarker+" "):]
		params = params[:i]
	}
	m = readParams(m, params)
	if tail != "" {
		m = applySDCPP(m, tail)
	}
	m = sanitize(m)
	if m.Empty() {
		return Metadata{}
	}
	return m
}

// indexLineStart finds marker at the start of a line.
func indexLineStart(s, marker string) int {
	if strings.HasPrefix(s, marker) {
		return 0
	}
	if i := strings.Index(s, "\n"+marker); i >= 0 {
		return i + 1
	}
	return -1
}

func readParams(m Metadata, line string) Metadata {
	idx := paramRe.FindAllStringSubmatchIndex(line, -1)
	for n, loc := range idx {
		key := line[loc[2]:loc[3]]
		end := len(line)
		if n+1 < len(idx) {
			end = idx[n+1][0]
		}
		val := strings.TrimSpace(line[loc[1]:end])
		switch key {
		case "Steps":
			if v, err := strconv.Atoi(val); err == nil {
				m.Steps = &v
			}
		case "CFG scale":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				m.CfgScale = &v
			}
		case "Seed":
			if v, err := strconv.ParseInt(val, 10, 64); err == nil {
				m.Seed = &v
			}
		case "Size":
			var w, h int
			if _, err := parseSize(val, &w, &h); err == nil {
				m.Width, m.Height = &w, &h
			}
		case "Model":
			m.Model = val
		case "Sampler":
			m.Sampler = val
		case "Denoising strength":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				m.Strength = &v
			}
		}
	}
	return m
}

func parseSize(s string, w, h *int) (int, error) {
	parts := strings.SplitN(s, "x", 2)
	if len(parts) != 2 {
		return 0, strconv.ErrSyntax
	}
	a, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, err
	}
	b, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, err
	}
	*w, *h = a, b
	return 2, nil
}

// sdcppJSON is the part of stable-diffusion.cpp's "SDCPP:" JSON that is read.
type sdcppJSON struct {
	Seed     *json.Number `json:"seed"`
	Width    *int         `json:"width"`
	Height   *int         `json:"height"`
	Strength *float64     `json:"strength"`
	Prompt   struct {
		Positive *string `json:"positive"`
		Negative *string `json:"negative"`
	} `json:"prompt"`
	Models struct {
		Model string `json:"model"`
	} `json:"models"`
	Sampling struct {
		Method   string `json:"method"`
		Steps    *int   `json:"steps"`
		Guidance struct {
			TxtCfg *float64 `json:"txt_cfg"`
		} `json:"guidance"`
	} `json:"sampling"`
}

// applySDCPP lets the JSON tail win over the text for the fields it carries:
// it holds the exact prompts and seed.
func applySDCPP(m Metadata, tail string) Metadata {
	dec := json.NewDecoder(strings.NewReader(tail))
	dec.UseNumber()
	var j sdcppJSON
	if err := dec.Decode(&j); err != nil {
		return m
	}
	if j.Prompt.Positive != nil {
		m.Prompt = *j.Prompt.Positive
	}
	if j.Prompt.Negative != nil {
		m.NegativePrompt = *j.Prompt.Negative
	}
	if j.Seed != nil {
		if v, err := j.Seed.Int64(); err == nil {
			m.Seed = &v
		}
	}
	if j.Sampling.Steps != nil {
		m.Steps = j.Sampling.Steps
	}
	if j.Sampling.Guidance.TxtCfg != nil {
		m.CfgScale = j.Sampling.Guidance.TxtCfg
	}
	if j.Models.Model != "" {
		m.Model = j.Models.Model
	}
	if j.Width != nil {
		m.Width = j.Width
	}
	if j.Height != nil {
		m.Height = j.Height
	}
	if j.Strength != nil {
		m.Strength = j.Strength
	}
	if j.Sampling.Method != "" && m.Sampler == "" {
		m.Sampler = j.Sampling.Method
	}
	return m
}
