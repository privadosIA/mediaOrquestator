import json, os, glob, time
os.environ["HF_HUB_OFFLINE"] = "1"
import numpy as np, soundfile as sf, torch
from transformers import ClapModel, ClapProcessor
R = os.environ.get("CLAP", "laion/clap-htsat-unfused")
t0 = time.time()
m = ClapModel.from_pretrained(R).eval(); p = ClapProcessor.from_pretrained(R)
print("load s", round(time.time() - t0, 1), flush=True)
vocab = json.load(open("/work/vocab.json"))
vocab["genre"] = [g for g in vocab["genre"] if g not in ("k-pop", "j-pop", "c-pop")]
T = {"genre": "This audio is a {} song.", "mood": "This audio is a {} song.", "instrument": "This audio features {}.",
     "era": "This audio is a song in {} style.", "production": "This audio is a {} recording.",
     "tempo": "This audio is a {} song.", "vocal": "This audio has {}."}
def norm(x): return x / x.norm(dim=-1, keepdim=True)
with torch.no_grad():
    emb = {}
    for cat, tags in vocab.items():
        ti = p(text=[T[cat].format(t) for t in tags], return_tensors="pt", padding=True)
        emb[cat] = (tags, norm(m.get_text_features(**ti)))
    for f in sorted(glob.glob("/work/clips48/*.wav")):
        w, sr = sf.read(f, dtype="float32"); assert sr == 48000
        t1 = time.time()
        segs = [w[i:i + 480000] for i in (0, 240000, 480000) if len(w[i:i + 480000]) > 48000]
        ai = p(audios=segs, sampling_rate=48000, return_tensors="pt")
        a = norm(m.get_audio_features(**ai)).mean(0, keepdim=True)
        print("\n==", os.path.basename(f), f"({time.time() - t1:.1f}s)")
        for cat, (tags, te) in emb.items():
            sim = (a @ te.T)[0]; top = sim.topk(3)
            print(f"  {cat:10s}", ", ".join(f"{tags[i]} {v:.2f}" for v, i in zip(top.values.tolist(), top.indices.tolist())))
