import json, os, sys, time, glob
os.environ["HF_HUB_OFFLINE"] = "1"
import torch, soundfile as sf
from muq import MuQMuLan
t0 = time.time()
m = MuQMuLan.from_pretrained("OpenMuQ/MuQ-MuLan-large", cache_dir="/ckpt").eval()
print("load s", round(time.time() - t0, 1), flush=True)
vocab = json.load(open("/work/vocab.json"))
vocab["genre"] = [g for g in vocab["genre"] if g not in ("k-pop","j-pop","c-pop")]
T = {"genre":"This is a {} song.","mood":"A {} song.","instrument":"A song featuring {}.","era":"A song in {} style.","production":"A {} recording.","tempo":"A {} song.","vocal":"A song with {}."}
with torch.no_grad():
    emb = {}
    for cat, tags in vocab.items():
        texts = [T[cat].format(t) for t in tags]
        _unused = [f"{t} music" if cat in ("genre", "instrument", "production", "era") else t for t in tags]
        if cat == "mood": texts = [f"{t} music" for t in tags]
        if cat == "vocal": texts = [f"music with {t}" for t in tags]
        if cat == "tempo": texts = [f"{t} tempo music" for t in tags]
        emb[cat] = (tags, m(texts=texts))
    for f in sorted(glob.glob("/work/c9/w24/*.wav")):
        w, sr = sf.read(f, dtype="float32"); assert sr == 24000
        t1 = time.time()
        a = m(wavs=torch.tensor(w).unsqueeze(0))
        dt = time.time() - t1
        print("\n==", os.path.basename(f), f"({dt:.1f}s)")
        for cat, (tags, te) in emb.items():
            sim = m.calc_similarity(a, te)[0]
            top = sim.topk(3)
            print(f"  {cat:10s}", ", ".join(f"{tags[i]} {v:.2f}" for v, i in zip(top.values.tolist(), top.indices.tolist())))
