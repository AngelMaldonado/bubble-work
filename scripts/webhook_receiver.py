#!/usr/bin/env python3
"""Receptor de webhooks de Bubble Work para la suite live.

POST cualquier ruta: verifica X-Bubble-Signature (sha256=<hex>, HMAC-SHA256 del
cuerpo con BUBBLE_WEBHOOK_SECRET), escribe una línea JSONL en `--log` con
{valid, event, delivery, body} y contesta 204, o 401 si la firma no cuadra.

También es el ejemplo de lo que tiene que hacer un receptor de verdad (cuby.work):
recalcular la firma sobre el cuerpo crudo y usar `body.id` como llave de
idempotencia.
"""
import argparse
import hashlib
import hmac
import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

SECRET = os.environ.get("BUBBLE_WEBHOOK_SECRET", "")
LOG = ""


def valid(body: bytes, signature: str) -> bool:
    want = "sha256=" + hmac.new(SECRET.encode(), body, hashlib.sha256).hexdigest()
    return bool(SECRET) and hmac.compare_digest(want.encode(), signature.encode())


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0") or 0)
        body = self.rfile.read(length) if length > 0 else b""
        ok = valid(body, self.headers.get("X-Bubble-Signature", ""))
        try:
            parsed = json.loads(body)
        except (ValueError, UnicodeError):
            parsed = {}
        if LOG:
            with open(LOG, "a") as log:
                log.write(json.dumps({
                    "valid": ok,
                    "event": self.headers.get("X-Bubble-Event", ""),
                    "delivery": self.headers.get("X-Bubble-Delivery", ""),
                    "body": parsed if isinstance(parsed, dict) else {},
                }) + "\n")
        self.send_response(204 if ok else 401)
        self.end_headers()

    def log_message(self, *args):
        pass


def main():
    global LOG
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("port", type=int)
    parser.add_argument("--log", default="")
    args = parser.parse_args()
    LOG = args.log
    HTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
