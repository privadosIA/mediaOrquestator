"""Tests of the service's API without a GPU: the generation is replaced by a stub.

    python3 -m unittest docker/heartmula/test_server.py
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
os.environ["HEARTMULA_JOBS"] = tempfile.mkdtemp()
os.environ["HEARTMULA_CKPT"] = tempfile.mkdtemp()
import server  # noqa: E402


def fake_generate(job):
    job.total = 4
    for i in range(4):
        job.done = i + 1
        time.sleep(0.01)
    if "fail" in job.request["lyrics"]:
        raise RuntimeError("out of memory")
    with open(job.path, "wb") as f:
        f.write(b"RIFFfake" + job.request["tags"].encode())


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
        self.fail("the job never finished")


class Tests(ApiTest):
    def test_health_reports_the_missing_weights(self):
        status, health = self.call("GET", "/health")
        self.assertEqual(status, 200)
        self.assertFalse(health["ready"])
        self.assertIn("HeartMuLa-oss-3B", health["missing"])
        self.assertEqual(health["version"], server.VERSION)

    def test_a_job_runs_and_its_audio_can_be_fetched(self):
        status, body = self.call("POST", "/generate", {"lyrics": "[Verse]\nhola", "tags": "Piano, Happy", "seed": 7})
        self.assertEqual(status, 202)
        view = self.wait(body["id"])
        self.assertEqual(view["state"], "done")
        self.assertEqual(view["progress"], {"done": 4, "total": 4})
        status, audio = self.call("GET", f"/jobs/{body['id']}/audio")
        self.assertEqual(status, 200)
        self.assertTrue(audio.startswith(b"RIFF"))
        self.assertTrue(audio.endswith(b"piano,happy"), "the tags are normalised")

    def test_defaults_and_limits(self):
        req = server.Engine.validate({"lyrics": "x", "tags": ["Piano", "happy"]})
        self.assertEqual((req["max_ms"], req["topk"], req["cfg_scale"], req["temperature"], req["seed"]), (240000, 50, 3.0, 1.0, 0))
        self.assertEqual(req["tags"], "piano,happy")
        for bad in (
            {"lyrics": "", "tags": "a"},
            {"lyrics": "x", "tags": ""},
            {"lyrics": "x", "tags": "a", "max_ms": 10},
            {"lyrics": "x", "tags": "a", "temperature": 5},
            {"lyrics": "x", "tags": "a", "seed": -1},
            {"lyrics": "x" * 20001, "tags": "a"},
            {"lyrics": "x", "tags": "a", "topk": True},
        ):
            with self.assertRaises(ValueError, msg=str(bad)[:60]):
                server.Engine.validate(bad)
        status, err = self.call("POST", "/generate", {"lyrics": "x"})
        self.assertEqual(status, 400)
        self.assertIn("tags", err["error"])

    def test_a_failed_job_is_reported_and_the_service_goes_on(self):
        _, body = self.call("POST", "/generate", {"lyrics": "fail please", "tags": "a"})
        view = self.wait(body["id"])
        self.assertEqual(view["state"], "failed")
        self.assertIn("out of memory", view["error"])
        status, err = self.call("GET", f"/jobs/{body['id']}/audio")
        self.assertEqual(status, 409)
        _, ok = self.call("POST", "/generate", {"lyrics": "fine", "tags": "a"})
        self.assertEqual(self.wait(ok["id"])["state"], "done")

    def test_jobs_run_one_at_a_time_in_order(self):
        ids = [self.call("POST", "/generate", {"lyrics": f"song {i}", "tags": "a"})[1]["id"] for i in range(3)]
        for i in ids:
            self.assertEqual(self.wait(i)["state"], "done")

    def test_forgetting_a_job_deletes_its_audio(self):
        _, body = self.call("POST", "/generate", {"lyrics": "x", "tags": "a"})
        self.wait(body["id"])
        self.assertEqual(self.call("DELETE", f"/jobs/{body['id']}")[0], 200)
        self.assertEqual(self.call("GET", f"/jobs/{body['id']}")[0], 404)

    def test_unknown_things_are_404(self):
        self.assertEqual(self.call("GET", "/jobs/abc123")[0], 404)
        self.assertEqual(self.call("GET", "/nothing")[0], 404)


class TokenTests(ApiTest):
    token = "secret"

    def test_every_call_needs_the_token(self):
        self.assertEqual(self.call("GET", "/health")[0], 401)
        self.assertEqual(self.call("POST", "/generate", {"lyrics": "x", "tags": "a"})[0], 401)
        self.assertEqual(self.call("GET", "/health", headers={"Authorization": "Bearer wrong"})[0], 401)
        self.assertEqual(self.call("GET", "/health", headers={"Authorization": "Bearer secret"})[0], 200)


class SecretFileTest(unittest.TestCase):
    """The token may come from a file (a secret mounted by the container runtime), and a file that cannot be read stops the service naming the file."""

    def test_token_from_a_file(self):
        import tempfile
        with tempfile.NamedTemporaryFile("w", suffix=".tok", delete=False) as f:
            f.write("s3cret-value\n")
        try:
            os.environ["HEARTMULA_TOKEN_FILE"] = f.name
            self.assertEqual(server._secret("HEARTMULA_TOKEN"), "s3cret-value")
            os.environ["HEARTMULA_TOKEN_FILE"] = f.name + ".missing"
            with self.assertRaises(SystemExit) as raised:
                server._secret("HEARTMULA_TOKEN")
            self.assertIn(".missing", str(raised.exception))
            self.assertNotIn("s3cret-value", str(raised.exception))
        finally:
            os.environ.pop("HEARTMULA_TOKEN_FILE", None)
            os.unlink(f.name)


if __name__ == "__main__":
    unittest.main()
