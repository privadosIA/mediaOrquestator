package music

import "testing"

func TestDetectLanguage(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"spanish", "[Verse 1]\nEl amor que siento por ti\nno se acaba en la noche\n[Chorus]\nMi corazón es tuyo y yo soy tu vida", "es"},
		{"spanish with marks", "¿Dónde estás? Mañana vuelvo a casa, señor", "es"},
		{"english", "[Verse]\nI don't know where you are tonight\nBut my heart is with you and the love is in the air", "en"},
		{"french", "Je suis dans la nuit avec mon amour\nC'est la vie, nous sommes très loin et je pense à toi pour toujours", "fr"},
		{"german", "Ich bin allein in der Nacht\nund du bist nicht mit mir, mein Herz ist so müde", "de"},
		{"italian", "Sono qui con il mio amore\nnon ho paura della notte, il cuore è con te", "it"},
		{"portuguese", "Eu sou você e não tenho medo\nmeu coração está com você, minha vida é uma canção", "pt"},
		{"korean", "사랑해 너를 영원히 나의 마음은 너의 것", "ko"},
		{"japanese with kana", "愛してるよ、君のことをずっと忘れない", "ja"},
		{"chinese without kana", "我爱你,直到永远,我的心属于你", "zh"},
		{"russian", "Я люблю тебя, моё сердце с тобой всегда", "ru"},
		{"empty", "", "en"},
		{"only tags", "[Verse]\n[Chorus]\n[Outro]", "en"},
		{"too short", "la la la", "en"},
		{"mixed and unclear", "yeah el the amor la night", "en"},
	}
	for _, c := range cases {
		if got := DetectLanguage(c.text); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetectLanguageIgnoresSectionTagsAndIsDeterministic(t *testing.T) {
	// "[Intro]" etc. must not count as English words.
	es := "[Verse]\n[Chorus]\nEl amor que siento por ti no se acaba en la noche, mi corazón"
	for i := 0; i < 20; i++ {
		if got := DetectLanguage(es); got != "es" {
			t.Fatalf("run %d: got %q", i, got)
		}
	}
}
