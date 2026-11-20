"""Tests of the service's API without a GPU: the generation is replaced by a stub.

    python3 -m unittest docker/diffrhythm2/test_server.py
"""

import http.client
import json
import os
import sys
import tempfile
import threading
import time
import unittest

sys.path.insert(0, os.path.dirname(__file__))
os.environ["DIFFRHYTHM2_JOBS"] = tempfile.mkdtemp()
os.environ["DIFFRHYTHM2_CKPT"] = tempfile.mkdtemp()
import server  # noqa: E402


def fake_generate(job):
    job.total = 1
    time.sleep(0.01)
    if "fail" in job.request["lyrics"]:
        raise RuntimeError("out of memory")
    job.done = 1
    with open(job.path, "wb") as f:
        f.write(b"RIFFfake" + job.request["style"].encode())


class FakeModule:
    def __init__(self, name, calls):
        self.name, self.calls = name, calls

    def to(self, device):
        self.calls.append((self.name, device))
        return self


class RestInRamTest(unittest.TestCase):
    def test_the_models_go_to_the_card_for_a_song_and_come_back(self):
        calls = []
        mods = [FakeModule(n, calls) for n in "abc"]
        with server.Engine.on_gpu(mods, empty_cache=lambda: calls.append("empty")):
            calls.append("song")
        self.assertEqual(calls, [("a", "cuda"), ("b", "cuda"), ("c", "cuda"), "song",
                                 ("a", "cpu"), ("b", "cpu"), ("c", "cpu"), "empty"])

    def test_they_come_back_when_the_song_fails(self):
        calls = []
        mods = [FakeModule("a", calls)]
        with self.assertRaises(RuntimeError):
            with server.Engine.on_gpu(mods, empty_cache=lambda: calls.append("empty")):
                raise RuntimeError("out of memory")
        self.assertEqual(calls[-2:], [("a", "cpu"), "empty"])


class LyricsTest(unittest.TestCase):
    def test_tags_are_written_in_lower_case_and_start_is_added(self):
        out = server.normalize_lyrics("[Verse]\nhello there\n[Chorus]\nsing it\n[End]")
        self.assertEqual(out, "[start]\n[verse]\nhello there\n[chorus]\nsing it\n[end]")

    def test_unknown_tags_are_dropped(self):
        out = server.normalize_lyrics("[start]\n[Pre-Chorus]\nhello\n[Bridge]\nthere")
        self.assertEqual(out, "[start]\nhello\n[bridge]\nthere")

    def test_start_is_not_added_twice(self):
        out = server.normalize_lyrics("[Start]\n[verse]\nhello")
        self.assertEqual(out.count("[start]"), 1)

    def test_tags_alone_are_refused(self):
        with self.assertRaises(ValueError):
            server.normalize_lyrics("[verse]\n[chorus]\n")

    def test_blank_lines_go(self):
        self.assertEqual(server.normalize_lyrics("\n hello \n\n"), "[start]\nhello")


class ApiTest(unittest.TestCase):
    token = ""

    @classmethod
    def setUpClass(cls):
        server.TOKEN = cls.token
        cls.engine = server.Engine(generate=fake_generate)
        cls.srv = server.serve(cls.engine, port=0, host="127.0.0.1")
        cls.port = cls.srv.server_address[1]
        threading.Thread(target=cls.srv.serve_forever, daemon=True).start()

    @classmethod
    def tearDownClass(cls):
        cls.srv.shutdown()
        server.TOKEN = ""

    def call(self, method, path, body=None, headers=None):
        conn = http.client.HTTPConnection("127.0.0.1", self.port, timeout=5)
        data = json.dumps(body) if body is not None else None
        conn.request(method, path, data, headers or {})
        resp = conn.getresponse()
        raw = resp.read()
        ctype = resp.getheader("Content-Type")
        return resp.status, (json.loads(raw) if ctype == "application/json" else raw)

    def wait(self, job_id):
        for _ in range(200):
            status, view = self.call("GET", f"/jobs/{job_id}")
            if view["state"] in ("done", "failed"):
                return view
            time.sleep(0.02)
        self.fail("the job did not finish")

    def good(self, **extra):
        body = {"lyrics": "[verse]\nhello there\n[chorus]\nsing it", "style": "Pop, Piano", "seed": 7}
        body.update(extra)
        return body

    def test_a_song(self):
        status, out = self.call("POST", "/generate", self.good())
        self.assertEqual(status, 202)
        view = self.wait(out["id"])
        self.assertEqual(view["state"], "done")
        status, audio = self.call("GET", f"/jobs/{out['id']}/audio")
        self.assertEqual(status, 200)
        self.assertTrue(audio.startswith(b"RIFF"))
        self.assertEqual(self.call("DELETE", f"/jobs/{out['id']}")[0], 200)
        self.assertEqual(self.call("GET", f"/jobs/{out['id']}")[0], 404)

    def test_the_audio_is_not_there_before_the_end(self):
        job = server.Job({"lyrics": "x", "style": "x"})
        self.engine.jobs[job.id] = job
        self.assertEqual(self.call("GET", f"/jobs/{job.id}/audio")[0], 409)

    def test_a_failed_job_says_why(self):
        status, out = self.call("POST", "/generate", self.good(lyrics="[verse]\nplease fail"))
        view = self.wait(out["id"])
        self.assertEqual(view["state"], "failed")
        self.assertIn("out of memory", view["error"])

    def test_refusals_name_the_field(self):
        for body, field in [
            ({"style": "Pop"}, "lyrics"),
            ({"lyrics": "[verse]", "style": "Pop"}, "lyrics"),
            ({"lyrics": "hello"}, "style"),
            ({"lyrics": "hello", "style": "Pop", "seed": -1}, "seed"),
            ({"lyrics": "hello", "style": "Pop", "max_secs": 999}, "max_secs"),
            ({"lyrics": "hello", "style": "Pop", "steps": "many"}, "steps"),
        ]:
            status, out = self.call("POST", "/generate", body)
            self.assertEqual(status, 400, body)
            self.assertIn(field, out["error"])

    def test_the_style_is_tidied(self):
        req = server.Engine.validate(self.good(style="  Pop ,Piano,  Warm "))
        self.assertEqual(req["style"], "Pop, Piano, Warm")

    def test_defaults(self):
        req = server.Engine.validate({"lyrics": "hello", "style": "Pop"})
        self.assertEqual((req["max_secs"], req["cfg_strength"], req["steps"], req["seed"]), (210.0, 2.0, 16, 0))

    def test_health_lists_what_is_missing(self):
        status, h = self.call("GET", "/health")
        self.assertEqual(status, 200)
        self.assertFalse(h["ready"])
        self.assertIn("model.safetensors", h["missing"])
        self.assertEqual(h["version"], server.VERSION)

    def test_unknown_routes(self):
        self.assertEqual(self.call("GET", "/nope")[0], 404)


class TokenTest(ApiTest):
    token = "secret"

    def test_without_the_token(self):
        self.assertEqual(self.call("GET", "/health")[0], 401)

    def test_with_the_token(self):
        status, _ = self.call("GET", "/health", headers={"Authorization": "Bearer secret"})
        self.assertEqual(status, 200)

    # The inherited tests call without a token: they only make sense without one.
    def _skip(self):
        self.skipTest("needs no token")

    test_a_song = test_the_audio_is_not_there_before_the_end = test_a_failed_job_says_why = _skip
    test_refusals_name_the_field = test_health_lists_what_is_missing = test_unknown_routes = _skip


class SecretFileTest(unittest.TestCase):
    """The token may come from a file (a secret mounted by the container runtime), and a file that cannot be read stops the service naming the file."""

    def test_token_from_a_file(self):
        import tempfile
        with tempfile.NamedTemporaryFile("w", suffix=".tok", delete=False) as f:
            f.write("s3cret-value\n")
        try:
            os.environ["DIFFRHYTHM2_TOKEN_FILE"] = f.name
            self.assertEqual(server._secret("DIFFRHYTHM2_TOKEN"), "s3cret-value")
            os.environ["DIFFRHYTHM2_TOKEN_FILE"] = f.name + ".missing"
            with self.assertRaises(SystemExit) as raised:
                server._secret("DIFFRHYTHM2_TOKEN")
            self.assertIn(".missing", str(raised.exception))
            self.assertNotIn("s3cret-value", str(raised.exception))
        finally:
            os.environ.pop("DIFFRHYTHM2_TOKEN_FILE", None)
            os.unlink(f.name)


if __name__ == "__main__":
    unittest.main()
