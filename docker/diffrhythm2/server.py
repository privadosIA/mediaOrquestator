#!/usr/bin/env python3
"""The DiffRhythm2 music engine as a small HTTP service.

Calls (JSON; an optional bearer token protects all of them):

  GET  /health               {ready, version, busy, queued, gpu: {name, free_mb, total_mb}, missing}
  POST /generate             {lyrics, style, max_secs, seed, cfg_strength, steps} -> 202 {id}
  GET  /jobs/{id}            {id, state, progress: {done, total}, error, seconds}
  GET  /jobs/{id}/audio      the song as a 48 kHz stereo WAV (409 until the job is done)
  DELETE /jobs/{id}          forgets a job and deletes its audio

One song is made at a time; the others wait in a queue. The model reads English (and Chinese) lyrics only and
has no instrumental mode. It has no seed option of its own: the service sets the random seed before every song.
`max_secs` is a ceiling: the model stops by itself at the end of the lyrics. The service never downloads
anything: the weights are in /ckpt (see fetch.py) and the libraries are told to stay offline. Standard library
only for the API (the model code is imported when the first song is made), so it can be tested without a GPU.
"""

import contextlib
import itertools
import json
import os
import queue
import re
import shutil
import sys
import tempfile
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

def _secret(name):
    """The value of the variable NAME, or the trimmed content of the file the variable NAME_FILE names (a secret mounted by the container runtime).
    A file that cannot be read stops the service and the message names the file, never the value."""
    path = os.environ.get(name + "_FILE", "")
    if path:
        try:
            with open(path, encoding="utf-8") as f:
                return f.read().strip()
        except OSError as e:
            raise SystemExit(f"cannot read {name}_FILE ({path}): {e.strerror}")
    return os.environ.get(name, "")


CKPT = os.environ.get("DIFFRHYTHM2_CKPT", "/ckpt")
TOKEN = _secret("DIFFRHYTHM2_TOKEN")
VERSION = os.environ.get("DIFFRHYTHM2_VERSION", "1")
PORT = int(os.environ.get("DIFFRHYTHM2_PORT", "8788"))
JOBS_DIR = os.environ.get("DIFFRHYTHM2_JOBS") or tempfile.mkdtemp(prefix="diffrhythm2-jobs-")   # a private folder (0700), not a fixed name in a shared one
UPSTREAM = os.environ.get("DIFFRHYTHM2_UPSTREAM", "/opt/dr2")
KEEP_JOBS = 20

# What the weights folder must hold: the model files and the hub folders of the three models it reads.
REQUIRED = [
    "config.json",
    "model.safetensors",
    "decoder.bin",
    "decoder.json",
    "models--OpenMuQ--MuQ-MuLan-large",
    "models--OpenMuQ--MuQ-large-msd-iter",
    "models--xlm-roberta-base",
]

LIMITS = {
    "max_secs": (10.0, 210.0, 210.0),
    "cfg_strength": (0.5, 10.0, 2.0),
    "steps": (4, 64, 16),
}

# The structure tags the model knows; any other tag line is dropped.
KNOWN_TAGS = {
    "start", "end", "intro", "verse", "chorus", "outro", "inst", "solo", "bridge", "hook", "break", "stop", "space",
}
TAG_LINE = re.compile(r"^\[(.*?)\]$")


def normalize_lyrics(text):
    """Writes the tags in lower case, drops the ones the model does not know and adds [start] when it is
    missing. Raises ValueError when there is no line of text (a song of tags alone has nothing to sing)."""
    lines, has_text, has_start = [], False, False
    for raw in text.splitlines():
        line = raw.strip()
        if not line:
            continue
        m = TAG_LINE.match(line)
        if m:
            tag = m.group(1).strip().lower()
            if tag in KNOWN_TAGS:
                lines.append(f"[{tag}]")
                has_start = has_start or tag == "start"
            continue
        lines.append(line)
        has_text = True
    if not has_text:
        raise ValueError("lyrics must have at least one line of text besides the structure tags")
    if not has_start:
        lines.insert(0, "[start]")
    return "\n".join(lines)


class Job:
    def __init__(self, request):
        self.id = uuid.uuid4().hex[:12]
        self.request = request
        self.state = "queued"
        self.done = 0
        self.total = 0
        self.error = ""
        self.created = time.time()
        self.started = 0.0
        self.finished = 0.0
        self.path = os.path.join(JOBS_DIR, self.id + ".wav")

    def view(self):
        end = self.finished or time.time()
        return {
            "id": self.id,
            "state": self.state,
            "progress": {"done": self.done, "total": self.total},
            "error": self.error,
            "seconds": round(end - self.started, 1) if self.started else 0,
        }


class Engine:
    """The queue, the worker and the model. `generate` can be replaced (the tests do)."""

    def __init__(self, generate=None):
        self.jobs = {}
        self.order = []
        self.lock = threading.Lock()
        self.queue = queue.Queue()
        self.current = None
        self._model = None
        self.generate = generate or self._generate_with_model
        os.makedirs(JOBS_DIR, exist_ok=True)
        threading.Thread(target=self._worker, daemon=True).start()

    # --- requests -----------------------------------------------------------------

    @staticmethod
    def validate(body):
        """Turns a request body into the request the worker uses, or raises ValueError with the reason."""
        if not isinstance(body, dict):
            raise ValueError("the body must be a JSON object")
        lyrics = body.get("lyrics")
        if not isinstance(lyrics, str) or not lyrics.strip():
            raise ValueError("lyrics must be a non-empty text")
        if len(lyrics) > 20_000:
            raise ValueError("lyrics are too long (20000 characters at most)")
        style = body.get("style", "")
        if isinstance(style, list):
            style = ", ".join(str(t) for t in style)
        if not isinstance(style, str):
            raise ValueError("style must be a text or a list of words")
        style = ", ".join(p.strip() for p in style.strip().split(",")).strip(", ")
        if not style:
            raise ValueError("style must name at least one style")
        if len(style) > 500:
            raise ValueError("style is too long (500 characters at most)")
        req = {"lyrics": normalize_lyrics(lyrics), "style": style}
        for key, (lo, hi, default) in LIMITS.items():
            value = body.get(key, default)
            if isinstance(value, bool) or not isinstance(value, (int, float)):
                raise ValueError(f"{key} must be a number")
            if not lo <= value <= hi:
                raise ValueError(f"{key} must be between {lo} and {hi}")
            req[key] = int(value) if key == "steps" else float(value)
        seed = body.get("seed", 0)
        if isinstance(seed, bool) or not isinstance(seed, int) or not 0 <= seed < 2**63:
            raise ValueError("seed must be a whole number, 0 or more")
        req["seed"] = seed
        return req

    def submit(self, body):
        job = Job(self.validate(body))
        with self.lock:
            self.jobs[job.id] = job
            self.order.append(job.id)
            self._trim()
        self.queue.put(job)
        return job

    def get(self, job_id):
        with self.lock:
            return self.jobs.get(job_id)

    def forget(self, job_id):
        with self.lock:
            job = self.jobs.pop(job_id, None)
            if job_id in self.order:
                self.order.remove(job_id)
        if job:
            try:
                os.remove(job.path)
            except OSError:
                pass
        return job is not None

    def _trim(self):
        """Keeps the last KEEP_JOBS finished jobs (called with the lock held)."""
        finished = [i for i in self.order if self.jobs[i].state in ("done", "failed")]
        for old in finished[: max(0, len(finished) - KEEP_JOBS)]:
            job = self.jobs.pop(old)
            self.order.remove(old)
            try:
                os.remove(job.path)
            except OSError:
                pass

    def queued(self):
        with self.lock:
            return sum(1 for j in self.jobs.values() if j.state == "queued")

    # --- the worker ---------------------------------------------------------------

    def _worker(self):
        while True:
            job = self.queue.get()
            if job.id not in self.jobs:  # forgotten while it waited
                continue
            self.current = job
            job.state, job.started = "running", time.time()
            try:
                self.generate(job)
                job.state = "done"
            except Exception as e:  # reported to the client, the service goes on
                job.state, job.error = "failed", f"{type(e).__name__}: {e}"
            finally:
                job.finished = time.time()
                self.current = None

    def _load(self):
        """Loads the models once, from /ckpt and offline, the way the upstream inference script does."""
        if self._model is not None:
            return self._model
        os.environ["HF_HUB_OFFLINE"] = "1"
        os.environ["TRANSFORMERS_OFFLINE"] = "1"
        if UPSTREAM not in sys.path:
            sys.path.insert(0, UPSTREAM)
        import torch
        from muq import MuQMuLan
        from safetensors.torch import load_file

        import inference as dr2  # the upstream script, as a module: its main is guarded
        from bigvgan.model import Generator
        from diffrhythm2.backbones.dit import DiT
        from diffrhythm2.cfm import CFM

        with open(os.path.join(CKPT, "config.json")) as f:
            cfg = json.load(f)
        cfg["use_flex_attn"] = False
        model = CFM(transformer=DiT(**cfg), num_channels=cfg["mel_dim"], block_size=cfg["block_size"])
        model.load_state_dict(load_file(os.path.join(CKPT, "model.safetensors")))
        model = model.half()  # half in RAM: the file is F32; the card gets it only while a song is made (on_gpu)
        mulan = MuQMuLan.from_pretrained("OpenMuQ/MuQ-MuLan-large", cache_dir=CKPT)
        dr2.lrc_tokenizer = dr2.CNENTokenizer()
        decoder = Generator(os.path.join(CKPT, "decoder.json"), os.path.join(CKPT, "decoder.bin")).half()
        self._model = (dr2, model, mulan, decoder)
        return self._model

    @staticmethod
    @contextlib.contextmanager
    def on_gpu(modules, device="cuda", empty_cache=None):
        """The models rest in RAM between songs: they go to the card for one song and come back after it, so
        two containers up at once take no video memory while nobody is making a song."""
        for m in modules:
            m.to(device)
        try:
            yield
        finally:
            for m in modules:
                m.to("cpu")
            if empty_cache:
                empty_cache()

    def _generate_with_model(self, job):
        import torch

        dr2, model, mulan, decoder = self._load()
        with self.on_gpu((model, mulan, decoder), empty_cache=torch.cuda.empty_cache):
            self._sing(job, dr2, model, mulan, decoder)

    def _sing(self, job, dr2, model, mulan, decoder):
        import numpy as np
        import pedalboard
        import torch

        req = job.request
        device = torch.device("cuda")
        lyrics_token = dr2.parse_lyrics(req["lyrics"])
        lyrics_token = torch.tensor(list(itertools.chain.from_iterable(lyrics_token)), dtype=torch.long, device=device)
        with torch.no_grad():
            style = mulan(texts=[req["style"]]).to(device).squeeze(0).half()
        job.total = 1
        torch.manual_seed(req["seed"])
        torch.cuda.manual_seed_all(req["seed"])
        with torch.inference_mode():
            latent = model.sample_block_cache(
                text=lyrics_token.unsqueeze(0),
                duration=int(req["max_secs"] * 5),
                style_prompt=style.unsqueeze(0),
                steps=req["steps"],
                cfg_strength=req["cfg_strength"],
                process_bar=False,
            )
            latent = latent.transpose(1, 2)
            audio = decoder.decode_audio(latent, overlap=5, chunk_size=20)
            audio = audio.float().cpu().numpy().squeeze()[None, :]
        audio = np.ascontiguousarray(dr2.make_fake_stereo(audio, decoder.h.sampling_rate))
        with pedalboard.io.AudioFile(job.path, "w", decoder.h.sampling_rate, 2) as f:
            f.write(audio)
        job.done = 1

    # --- health -------------------------------------------------------------------

    def health(self):
        missing = [f for f in REQUIRED if not os.path.exists(os.path.join(CKPT, f))]
        gpu = None
        try:
            import torch

            if torch.cuda.is_available():
                free, total = torch.cuda.mem_get_info()
                gpu = {"name": torch.cuda.get_device_name(0), "free_mb": free // 2**20, "total_mb": total // 2**20}
        except Exception:
            gpu = None
        return {
            "ready": not missing and gpu is not None,
            "version": VERSION,
            "busy": self.current is not None,
            "queued": self.queued(),
            "gpu": gpu,
            "missing": missing,
        }


class Handler(BaseHTTPRequestHandler):
    engine = None

    def log_message(self, fmt, *args):  # one line per call on stderr is enough
        pass

    def _send(self, status, payload):
        data = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _authorized(self):
        if not TOKEN:
            return True
        return self.headers.get("Authorization", "") == f"Bearer {TOKEN}"

    def _route(self, method):
        if not self._authorized():
            return self._send(401, {"error": "a bearer token is required"})
        path = self.path.split("?")[0].rstrip("/")
        if method == "GET" and path == "/health":
            return self._send(200, self.engine.health())
        if method == "POST" and path == "/generate":
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if length > 1_000_000:
                    return self._send(413, {"error": "the request is too large"})
                body = json.loads(self.rfile.read(length) or b"{}")
                job = self.engine.submit(body)
            except (ValueError, json.JSONDecodeError) as e:
                return self._send(400, {"error": str(e)})
            return self._send(202, {"id": job.id})
        m = re.fullmatch(r"/jobs/([0-9a-f]+)(/audio)?", path)
        if m:
            job = self.engine.get(m.group(1))
            if job is None:
                return self._send(404, {"error": "unknown job"})
            if method == "DELETE" and not m.group(2):
                self.engine.forget(job.id)
                return self._send(200, {"ok": True})
            if method == "GET" and not m.group(2):
                return self._send(200, job.view())
            if method == "GET" and m.group(2):
                if job.state != "done":
                    return self._send(409, {"error": f"the job is {job.state}"})
                try:
                    size = os.path.getsize(job.path)
                    self.send_response(200)
                    self.send_header("Content-Type", "audio/wav")
                    self.send_header("Content-Length", str(size))
                    self.end_headers()
                    with open(job.path, "rb") as f:
                        shutil.copyfileobj(f, self.wfile)
                except OSError:
                    return self._send(410, {"error": "the audio is gone"})
                return None
        return self._send(404, {"error": "not found"})

    def do_GET(self):
        self._route("GET")

    def do_POST(self):
        self._route("POST")

    def do_DELETE(self):
        self._route("DELETE")


def serve(engine=None, port=PORT, host="0.0.0.0"):
    Handler.engine = engine or Engine()
    server = ThreadingHTTPServer((host, port), Handler)
    return server


if __name__ == "__main__":
    srv = serve()
    print(f"diffrhythm2 service {VERSION} listening on :{PORT} (weights: {CKPT})", flush=True)
    srv.serve_forever()
