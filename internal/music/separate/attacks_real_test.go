//go:build real

package separate

import (
	"os"
	"testing"
)

// With REAL_STEM set to a 16 kHz mono voice stem (the vocals-16k.wav of a piece) checks that a sung stem gives about one attack per
// syllable: between 150 and 400 for 120 seconds of a song of 129 words.
func TestReal_AttacksOfASungStem(t *testing.T) {
	path := os.Getenv("REAL_STEM")
	if path == "" {
		t.Skip("set REAL_STEM to a vocals-16k.wav")
	}
	env, attacks, err := AnalyseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	secs := float64(len(env.Voiced)) * env.Hop
	t.Logf("%.0f s of stem, %d attacks, voiced %.0f %%", secs, len(attacks), 100*env.VoicedShare())
	per120 := float64(len(attacks)) * 120 / secs
	if per120 < 150 || per120 > 400 {
		t.Fatalf("%d attacks (%.0f per 120 s), want between 150 and 400", len(attacks), per120)
	}
	for i := 1; i < len(attacks); i++ {
		if attacks[i]-attacks[i-1] < MinAttackGap-1e-9 {
			t.Fatalf("attacks %.3f and %.3f are closer than %.2f s", attacks[i-1], attacks[i], MinAttackGap)
		}
	}
}
