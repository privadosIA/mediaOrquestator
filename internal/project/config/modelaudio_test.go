package config

import (
	"strings"
	"testing"
)

func TestModelAudio_OnlyForH3AndLTX(t *testing.T) {
	c := ltxConfig(t)
	c.Defaults.Animate.ModelAudio = true
	if err := c.Validate(); err != nil {
		t.Errorf("model_audio with ltx-2.3 refused: %v", err)
	}
	c = miniMaxConfig(t)
	c.Defaults.Animate.ModelAudio = true
	if err := c.Validate(); err != nil {
		t.Errorf("model_audio with minimax-h3 refused: %v", err)
	}
	for _, engine := range []string{"", EngineWan, EngineAnimateDiff} {
		c = animateConfig(t)
		c.Defaults.Animate.Engine = engine
		c.Defaults.Animate.ModelAudio = true
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "model_audio only applies") {
			t.Errorf("engine %q: err = %v", engine, err)
		}
	}
}

func TestModelAudio_AbsentStaysAbsent(t *testing.T) {
	c := ltxConfig(t)
	if c.Defaults.Animate.ModelAudio {
		t.Error("model_audio must be off when unset")
	}
}

func TestModelAudioSpeech_NeedsModelAudio(t *testing.T) {
	c := ltxConfig(t)
	c.Defaults.Animate.ModelAudioSpeech = true
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "model_audio_speech needs") {
		t.Errorf("err = %v", err)
	}
	c.Defaults.Animate.ModelAudio = true
	if err := c.Validate(); err != nil {
		t.Errorf("model_audio_speech with model_audio refused: %v", err)
	}
}
