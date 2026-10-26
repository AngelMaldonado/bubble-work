#!/usr/bin/env python3
"""Pruebas del receptor simulado, sin Docker ni Bubble."""
import hashlib
import hmac
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import time
import unittest
import urllib.error
import urllib.request

SECRET = "bfs_test_only"
VIEW_TOKEN = "view_test_only"


class ReceiverTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        cls.url = f"http://127.0.0.1:{port}"
        cls.process = subprocess.Popen(
            [sys.executable, str(Path(__file__).with_name("fury_receiver.py")), str(port)],
            env={**os.environ, "BUBBLE_FURY_SECRET": SECRET, "BUBBLE_FURY_VIEW_TOKEN": VIEW_TOKEN,
                 "BUBBLE_FURY_HOST": "127.0.0.1", "BUBBLE_FURY_HEADLESS": "1"},
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            for _ in range(50):
                try:
                    cls.call("/health")
                    return
                except urllib.error.URLError:
                    time.sleep(0.05)
            raise RuntimeError("el receptor no arrancó")
        except Exception:
            cls.process.terminate()
            cls.process.wait(timeout=5)
            raise

    @classmethod
    def tearDownClass(cls):
        cls.process.terminate()
        cls.process.wait(timeout=5)

    @classmethod
    def call(cls, path, body=None, headers=None):
        req = urllib.request.Request(cls.url + path, data=body, headers=headers or {})
        try:
            response = urllib.request.urlopen(req, timeout=3)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            data = response.read()
            return response.code, json.loads(data) if data else None, response.headers

    def light(self, **changes):
        return {"type": "fury.changed", "person": self.id(), "email": "demo@example.test",
                "name": "Demo", "level": 3, "mood": "Desatada", "color": "red", "blink": True,
                "reasons": ["Prueba"], "at": "2026-10-23T22:00:00Z", **changes}

    def signed(self, body):
        return {"X-Bubble-Signature": "sha256=" + hmac.new(SECRET.encode(), body, hashlib.sha256).hexdigest()}

    def test_health_does_not_expose_people(self):
        status, body, headers = self.call("/health")
        self.assertEqual(status, 200)
        self.assertEqual(body, {"ok": True, "mode": "simulated-gateway"})
        self.assertEqual(headers["Cache-Control"], "no-store")

    def test_state_requires_separate_token(self):
        for token in ("", "Bearer wrong", "Bearer " + SECRET):
            self.assertEqual(self.call("/state", headers={"Authorization": token})[0], 401)

    def test_valid_signature_stores_light(self):
        light = self.light()
        body = json.dumps(light).encode()
        self.assertEqual(self.call("/fury", body, self.signed(body))[0], 204)
        status, result, _ = self.call("/state", headers={"Authorization": "Bearer " + VIEW_TOKEN})
        self.assertEqual(status, 200)
        self.assertIn(light, result["lights"])

    def test_signature_is_over_raw_bytes(self):
        body = json.dumps(self.light(), indent=2).encode()
        self.assertEqual(self.call("/fury", body, self.signed(body))[0], 204)

    def test_wrong_signature_does_not_store_light(self):
        light = self.light()
        body = json.dumps(light).encode()
        self.assertEqual(self.call("/fury", body, {"X-Bubble-Signature": "sha256=bad"})[0], 401)
        _, result, _ = self.call("/state", headers={"Authorization": "Bearer " + VIEW_TOKEN})
        self.assertNotIn(light, result["lights"])

    def test_signed_invalid_json_is_rejected(self):
        body = b"not-json"
        self.assertEqual(self.call("/fury", body, self.signed(body))[0], 400)
        self.assertEqual(self.call("/health")[0], 200)

    def test_signed_invalid_schema_is_rejected(self):
        for light in ([], {}, self.light(level=8), self.light(level=True), self.light(reasons=None)):
            body = json.dumps(light).encode()
            self.assertEqual(self.call("/fury", body, self.signed(body))[0], 400)

    def test_body_size_is_bounded(self):
        body = b"x" * 65537
        self.assertEqual(self.call("/fury", body, self.signed(body))[0], 413)

    def test_unknown_routes_are_rejected(self):
        self.assertEqual(self.call("/unknown")[0], 404)
        self.assertEqual(self.call("/unknown", b"{}")[0], 404)


if __name__ == "__main__":
    unittest.main()
