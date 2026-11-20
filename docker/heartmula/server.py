#!/usr/bin/env python3
"""The HeartMuLa music engine as a small HTTP service.

Calls (JSON; an optional bearer token protects all of them):

  GET  /health               {ready, version, busy, queued, gpu: {name, free_mb, total_mb}}
  POST /generate             {lyrics, tags, max_ms, seed, temperature, topk, cfg_scale} -> 202 {id}
  GET  /jobs/{id}            {id, state, progress: {done, total}, error, seconds}
  GET  /jobs/{id}/audio      the song as a 48 kHz stereo WAV (409 until the job is done)
  DELETE /jobs/{id}          forgets a job and deletes its audio

One song is made at a time; the others wait in a queue. The model stages are loaded on demand and freed after
each song (the library's lazy_load), so the GPU is free between songs. The model has no seed option of its
own: the service sets the random seed before every song. Standard library only (the model code is imported
when the first song is made), so the API can be tested without a GPU.
"""

import json
import os
import queue
import re
import shutil
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


CKPT = os.environ.get("HEARTMULA_CKPT", "/ckpt")
TOKEN = _secret("HEARTMULA_TOKEN")
VERSION = os.environ.get("HEARTMULA_VERSION", "1")
PORT = int(os.environ.get("HEARTMULA_PORT", "8787"))
JOBS_DIR = os.environ.get("HEARTMULA_JOBS") or tempfile.mkdtemp(prefix="heartmula-jobs-")   # a private folder (0700), not a fixed name in a shared one
KEEP_JOBS = 20

# The files the weights folder must have (the layout of `hf download` in the library's README).
REQUIRED = ["config.json", "gen_config.json", "tokenizer.json", "HeartMuLa-oss-3B", "HeartCodec-oss"]

LIMITS = {
    "max_ms": (5_000, 360_000, 240_000),
    "temperature": (0.1, 2.0, 1.0),
    "topk": (1, 500, 50),
    "cfg_scale": (1.0, 10.0, 3.0),
}


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
        self._pipe = None
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
        tags = body.get("tags", "")
        if isinstance(tags, list):
            tags = ",".join(str(t) for t in tags)
        if not isinstance(tags, str):
            raise ValueError("tags must be a text or a list of words")
        tags = ",".join(p.strip() for p in tags.strip().lower().split(",")).strip(",")
        if not tags:
            raise ValueError("tags must name at least one style")
        req = {"lyrics": lyrics, "tags": tags}
        for key, (lo, hi, default) in LIMITS.items():
            value = body.get(key, default)
            if isinstance(value, bool) or not isinstance(value, (int, float)):
                raise ValueError(f"{key} must be a number")
            if not lo <= value <= hi:
                raise ValueError(f"{key} must be between {lo} and {hi}")
            req[key] = int(value) if key in ("max_ms", "topk") else float(value)
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
        if self._pipe is None:
            import torch
            from heartlib import HeartMuLaGenPipeline

            device = torch.device("cuda")
            # The stages are loaded when a song needs them and freed after it (lazy_load). The decoder runs
            # in bf16: in fp32 it peaks at 14.6 GB at the end of a song, in bf16 at 6.9 GB.
            self._pipe = HeartMuLaGenPipeline.from_pretrained(
                CKPT,
                device={"mula": device, "codec": device},
                dtype={"mula": torch.bfloat16, "codec": torch.bfloat16},
                version="3B",
                lazy_load=True,
            )
        return self._pipe

    def _generate_with_model(self, job):
        import torch
        import heartlib.pipelines.music_generation as mg

        pipe = self._load()
        req = job.request

        def progress(iterable, *args, **kwargs):
            job.total = len(iterable) if hasattr(iterable, "__len__") else 0
            for i, item in enumerate(iterable):
                job.done = i + 1
                yield item

        mg.tqdm = progress  # the library reports its frames through tqdm: this is its progress
        torch.manual_seed(req["seed"])
        torch.cuda.manual_seed_all(req["seed"])
        with torch.no_grad():
            pipe(
                {"lyrics": req["lyrics"], "tags": req["tags"]},
                max_audio_length_ms=req["max_ms"],
                save_path=job.path,
                topk=req["topk"],
                temperature=req["temperature"],
                cfg_scale=req["cfg_scale"],
            )
        torch.cuda.empty_cache()

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
    print(f"heartmula service {VERSION} listening on :{PORT} (weights: {CKPT})", flush=True)
    srv.serve_forever()
