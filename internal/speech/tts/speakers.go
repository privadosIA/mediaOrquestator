package tts

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
)

// Speaker is one of the voices a multi-speaker Piper model holds.
type Speaker struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Speakers reads the speakers of a Piper voice from its .onnx.json companion. A voice with one speaker
// (the usual case), or whose config cannot be read, has none to choose: it returns nil. The names come
// from the model's speaker_id_map when it has one, else they are the ids.
func Speakers(voicePath string) []Speaker {
	data, err := os.ReadFile(voicePath + ".json")
	if err != nil {
		return nil
	}
	var cfg struct {
		NumSpeakers  int            `json:"num_speakers"`
		SpeakerIDMap map[string]int `json:"speaker_id_map"`
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.NumSpeakers < 2 {
		return nil
	}
	names := map[int]string{}
	for name, id := range cfg.SpeakerIDMap {
		names[id] = name
	}
	out := make([]Speaker, 0, cfg.NumSpeakers)
	for id := 0; id < cfg.NumSpeakers; id++ {
		n, ok := names[id]
		if !ok {
			n = strconv.Itoa(id)
		}
		out = append(out, Speaker{ID: id, Name: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SpeakerCount is how many speakers a voice holds (1 for the usual voice or an unreadable config).
func SpeakerCount(voicePath string) int {
	if n := len(Speakers(voicePath)); n > 0 {
		return n
	}
	return 1
}
