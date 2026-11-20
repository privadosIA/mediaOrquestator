import glob, os, json, numpy as np, librosa
out = {}
for f in sorted(glob.glob("/work/c9/w24/*.wav")):
    y, sr = librosa.load(f, sr=22050, mono=True)
    id = os.path.basename(f)[:-4]
    S = np.abs(librosa.stft(y, n_fft=2048, hop_length=512))
    contrast = librosa.feature.spectral_contrast(S=S, sr=sr, n_bands=6, fmin=100.0)
    rolloff = librosa.feature.spectral_rolloff(S=S, sr=sr, roll_percent=0.85)[0]
    bw = librosa.feature.spectral_bandwidth(S=S, sr=sr)[0]
    zcr = librosa.feature.zero_crossing_rate(y, hop_length=512)[0]
    H, P = librosa.decompose.hpss(S)
    hp = float(np.sum(H**2) / (np.sum(P**2) + 1e-9))
    chroma = librosa.feature.chroma_stft(S=S**2, sr=sr)
    c = chroma.mean(1); c = c / c.sum(); ent = float(-(c * np.log2(c + 1e-12)).sum())
    onset = librosa.onset.onset_strength(y=y, sr=sr)
    # beat regularity: autocorrelation peak of the onset envelope
    ac = librosa.autocorrelate(onset - onset.mean()); ac = ac / (ac[0] + 1e-9)
    reg = float(ac[10:200].max())
    mf = librosa.feature.mfcc(S=librosa.power_to_db(librosa.feature.melspectrogram(S=S**2, sr=sr)), n_mfcc=13)
    out[id] = {
        "contrast": [round(float(v), 1) for v in contrast.mean(1)],
        "rolloff_hz": round(float(rolloff.mean())), "bandwidth_hz": round(float(bw.mean())), "zcr": round(float(zcr.mean()), 3),
        "harm_perc": round(hp, 2), "chroma_entropy": round(ent, 2), "beat_regularity": round(reg, 2),
        "mfcc_std": round(float(mf[1:].std(axis=1).mean()), 1),
    }
    print(id, json.dumps(out[id]))
json.dump(out, open("/work/c9/extra.json", "w"))
