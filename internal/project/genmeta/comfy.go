package genmeta

import (
	"encoding/json"
	"sort"
	"strconv"
)

type comfyNode struct {
	ClassType string                     `json:"class_type"`
	Inputs    map[string]json.RawMessage `json:"inputs"`
}

// FromComfyPrompt reads ComfyUI's "prompt" graph (or a {"prompt": graph}
// wrapper): the first sampler node gives the seed, steps and cfg, and its
// positive and negative links lead to the text of the encode nodes. Anything
// that does not match leaves the field unset.
func FromComfyPrompt(raw []byte) Metadata {
	if len(raw) > MaxValue*4 {
		return Metadata{}
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return Metadata{}
	}
	if inner, ok := wrapper["prompt"]; ok && len(wrapper) <= 3 {
		var g map[string]comfyNode
		if json.Unmarshal(inner, &g) == nil && len(g) > 0 {
			return comfyGraph(g)
		}
	}
	if len(wrapper) > MaxGraphNodes {
		return Metadata{}
	}
	g := make(map[string]comfyNode, len(wrapper))
	for id, v := range wrapper {
		var n comfyNode
		if json.Unmarshal(v, &n) == nil && n.ClassType != "" {
			g[id] = n
		}
	}
	return comfyGraph(g)
}

var comfySamplers = map[string]bool{"KSampler": true, "KSamplerAdvanced": true, "SamplerCustom": true}

func comfyGraph(g map[string]comfyNode) Metadata {
	if len(g) == 0 || len(g) > MaxGraphNodes {
		return Metadata{}
	}
	ids := make([]string, 0, len(g))
	for id := range g {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, ea := strconv.Atoi(ids[i])
		b, eb := strconv.Atoi(ids[j])
		if ea == nil && eb == nil {
			return a < b
		}
		return ids[i] < ids[j]
	})
	var m Metadata
	for _, id := range ids {
		n := g[id]
		if !comfySamplers[n.ClassType] {
			continue
		}
		m.Prompt = comfyText(g, n.Inputs["positive"], 0)
		m.NegativePrompt = comfyText(g, n.Inputs["negative"], 0)
		if v, ok := seedInput(n.Inputs); ok {
			m.Seed = &v
		}
		if v, ok := numberInput(n.Inputs, "steps"); ok {
			s := int(v)
			m.Steps = &s
		}
		if v, ok := numberInput(n.Inputs, "cfg"); ok {
			m.CfgScale = &v
		}
		if s, ok := stringInput(n.Inputs, "sampler_name"); ok {
			m.Sampler = s
		}
		m.Model = comfyModel(g, n.Inputs["model"], 0)
		break
	}
	m = sanitize(m)
	if m.Empty() {
		return Metadata{}
	}
	return m
}

// comfyLink decodes ["nodeId", outputIndex].
func comfyLink(raw json.RawMessage) (string, bool) {
	var l []json.RawMessage
	if json.Unmarshal(raw, &l) != nil || len(l) != 2 {
		return "", false
	}
	var id string
	if json.Unmarshal(l[0], &id) == nil {
		return id, true
	}
	var n float64
	if json.Unmarshal(l[0], &n) == nil {
		return strconv.Itoa(int(n)), true
	}
	return "", false
}

// comfyText follows links (at most a few hops) to the text of an encode node.
func comfyText(g map[string]comfyNode, raw json.RawMessage, depth int) string {
	if depth > 5 || len(raw) == 0 {
		return ""
	}
	id, ok := comfyLink(raw)
	if !ok {
		return ""
	}
	n, ok := g[id]
	if !ok {
		return ""
	}
	if t, ok := stringInput(n.Inputs, "text"); ok {
		return t
	}
	if l, ok := n.Inputs["text"]; ok {
		return comfyText(g, l, depth+1)
	}
	for _, k := range []string{"conditioning", "conditioning_1"} {
		if l, ok := n.Inputs[k]; ok {
			if t := comfyText(g, l, depth+1); t != "" {
				return t
			}
		}
	}
	return ""
}

func comfyModel(g map[string]comfyNode, raw json.RawMessage, depth int) string {
	if depth > 5 || len(raw) == 0 {
		return ""
	}
	id, ok := comfyLink(raw)
	if !ok {
		return ""
	}
	n, ok := g[id]
	if !ok {
		return ""
	}
	if s, ok := stringInput(n.Inputs, "ckpt_name"); ok {
		return s
	}
	if s, ok := stringInput(n.Inputs, "unet_name"); ok {
		return s
	}
	if l, ok := n.Inputs["model"]; ok {
		return comfyModel(g, l, depth+1)
	}
	return ""
}

func numberInput(in map[string]json.RawMessage, keys ...string) (float64, bool) {
	for _, k := range keys {
		var v float64
		if raw, ok := in[k]; ok && json.Unmarshal(raw, &v) == nil {
			return v, true
		}
	}
	return 0, false
}

func stringInput(in map[string]json.RawMessage, key string) (string, bool) {
	var s string
	if raw, ok := in[key]; ok && json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	return "", false
}

// seedInput reads the seed as an integer: a float64 would round a 64-bit seed.
func seedInput(in map[string]json.RawMessage) (int64, bool) {
	for _, k := range []string{"seed", "noise_seed"} {
		raw, ok := in[k]
		if !ok {
			continue
		}
		var n json.Number
		if json.Unmarshal(raw, &n) == nil {
			if v, err := n.Int64(); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}
