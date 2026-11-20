import os
import json, re, sys, urllib.request, time
S = os.environ.get("STUDY_DIR", ".")
SYSTEM = open(S + "/system.txt").read()
extra = json.load(open(S + "/c9/extra.json"))
ours = {}
for line in open(S + "/c9/ours.txt"):
    c = line.rstrip("\n").split("\t")
    if c[0] == "WORDS": ours.setdefault(c[1], {})["words"] = c[2]
    elif len(c) >= 12: ours.setdefault(c[0], {}).update(bpm=c[10], key=c[11])
muq = {}
cur = None
for line in open(S + "/c9/muq.txt"):
    m = re.match(r"== (\w+)\.wav", line)
    if m: cur = m.group(1); muq[cur] = {}
    elif cur:
        k, rest = line.split(None, 1)[0], line.split(None, 1)[1]
        muq[cur][k] = [x.rsplit(" ", 1)[0] for x in rest.strip().split(", ")]
truth = {l.split("\t")[0]: l.rstrip().split("\t")[2] for l in open(S + "/c9/list.tsv")}

def extra_words(e):
    w = []
    r = e["rolloff_hz"]
    if r < 2200: w.append("most of the energy is in the low-mid range (mellow, few upper harmonics)")
    elif r > 5000: w.append("lots of upper harmonics and brightness")
    hp = e["harm_perc"]
    if hp > 20: w.append("mostly sustained harmonic sound (pads, strings or piano) with little percussion")
    elif hp < 4: w.append("strongly percussive, drum-driven sound")
    else: w.append("a mix of sustained instruments and percussion")
    br = e["beat_regularity"]
    if br > 0.58: w.append("a very steady, regular pulse")
    elif br < 0.35: w.append("a loose, free pulse")
    if e["zcr"] > 0.1: w.append("noisy or distorted high end")
    return w

def facts(id, v):
    o = ours[id]
    b = "Lyrics: none recognised (the song may be instrumental or its voice could not be read)\n"
    if o["bpm"]: b += f"Tempo: about {o['bpm']} BPM\n"
    if o["key"]: b += f"Key: {o['key']}\n"
    snd = o["words"].split("; ") if o["words"] else []
    if v in ("extra", "both"): snd += extra_words(extra[id])
    if snd: b += "Sound (measured): " + "; ".join(snd) + "\n"
    if v in ("muq", "both"):
        g = ", ".join(muq[id]["genre"][:3]); i = ", ".join(muq[id]["instrument"][:3])
        b += f"A music-tagging model suggests (not always right): genres {g}; instruments {i}\n"
    return b

def ask(prompt):
    body = {"model": "qwen3:8b", "stream": False, "think": False, "keep_alive": 0,
            "options": {"num_gpu": 0, "temperature": 0.2, "num_predict": 120},
            "messages": [{"role": "system", "content": SYSTEM}, {"role": "user", "content": prompt}]}
    r = urllib.request.Request("http://127.0.0.1:11434/api/chat", json.dumps(body).encode(), {"Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(r, timeout=600))["message"]["content"].strip()

res = {}
for id in truth:
    for v in ("base", "extra", "muq", "both"):
        t = time.time(); f = facts(id, v)
        try: a = ask(f)
        except Exception as e: a = "ERR " + str(e)
        res[f"{id}|{v}"] = {"facts": f, "answer": a}
        print(f"{id:11s} {v:5s} {time.time()-t:4.0f}s {a}", flush=True)
json.dump(res, open(S + "/c9/llm_results.json", "w"), ensure_ascii=False, indent=1)
