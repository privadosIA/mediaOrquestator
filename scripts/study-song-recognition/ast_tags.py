import glob, os, torch, soundfile as sf, numpy as np
os.environ["HF_HUB_OFFLINE"] = "1"
from transformers import ASTFeatureExtractor, ASTForAudioClassification
R = "MIT/ast-finetuned-audioset-10-10-0.4593"
fe = ASTFeatureExtractor.from_pretrained(R); m = ASTForAudioClassification.from_pretrained(R).eval()
lab = m.config.id2label
# the AudioSet labels that name a genre, a mood or a family of instruments (the rest -- Speech, Singing, Music... -- are noise)
KEEP = {"Pop music","Hip hop music","Rock music","Heavy metal","Punk rock","Grunge","Progressive rock","Rock and roll","Psychedelic rock","Rhythm and blues","Soul music","Reggae","Country","Swing music","Bluegrass","Funk","Folk music","Middle Eastern music","Jazz","Disco","Classical music","Opera","Electronic music","House music","Techno","Dubstep","Drum and bass","Electronica","Electronic dance music","Ambient music","Trance music","Music of Latin America","Salsa music","Flamenco","Blues","Music for children","New-age music","Vocal music","A capella","Music of Africa","Afrobeat","Christian music","Gospel music","Music of Asia","Carnatic music","Music of Bollywood","Ska","Traditional music","Independent music","Soundtrack music","Theme music","Video game music","Christmas music","Wedding music","Happy music","Sad music","Tender music","Exciting music","Angry music","Scary music","Piano","Keyboard (musical)","Synthesizer","Organ","Guitar","Electric guitar","Acoustic guitar","Bass guitar","Violin, fiddle","Cello","Saxophone","Trumpet","Drum kit","Accordion","Banjo","Mandolin","Harp","Flute","Orchestra","String section","Choir","Male singing","Female singing","Rapping","Singing"}
for f in sorted(glob.glob("/work/c9/w16/*.wav")):
    w, sr = sf.read(f, dtype="float32"); assert sr == 16000
    segs = [w[i:i + 160000] for i in (0, 120000, 240000) if len(w[i:i + 160000]) > 32000]
    with torch.no_grad():
        p = torch.sigmoid(m(**fe(segs, sampling_rate=16000, return_tensors="pt")).logits).mean(0)
    ranked = sorted(((p[i].item(), lab[i]) for i in range(len(lab)) if lab[i] in KEEP), reverse=True)
    print(os.path.basename(f)[:-4], "|", ", ".join(f"{n} {s:.2f}" for s, n in ranked[:7]), flush=True)
