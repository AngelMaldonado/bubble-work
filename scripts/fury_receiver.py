#!/usr/bin/env python3
"""Receptor SIMULADO del webhook de furia; no controla hardware.

    BUBBLE_FURY_SECRET=bfs_… just fury listen [puerto]  # 8787 por omisión

POST /fury verifica X-Bubble-Signature: sha256=<hex> (HMAC-SHA256 del cuerpo
crudo). GET /health no revela datos. GET /state requiere un Bearer token
separado, BUBBLE_FURY_VIEW_TOKEN: nunca publica nombres/correos sin él.

Por omisión escucha sólo en loopback. El contenedor usa BUBBLE_FURY_HOST=
0.0.0.0 y BUBBLE_FURY_HEADLESS=1 para escribir eventos a stdout sin limpiar la
terminal. `--log <archivo>` escribe el registro JSONL que usa la suite live.

Este ejemplo NO implementa el radio, los ACKs de los nodos ni el watchdog:
el firmware real debe apagar la luz a los 15 minutos sin información fresca.
"""
import argparse
import hashlib
import hmac
import json
import os
import sys
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

SECRET = os.environ.get("BUBBLE_FURY_SECRET", "")
VIEW_TOKEN = os.environ.get("BUBBLE_FURY_VIEW_TOKEN", "")
HOST = os.environ.get("BUBBLE_FURY_HOST", "127.0.0.1")
HEADLESS = os.environ.get("BUBBLE_FURY_HEADLESS") == "1"
MAX_BODY = 65536
LOG = ""
PORT = 8787
lights = {}  # person -> último aviso; sólo memoria, se pierde al reiniciar


def valid(body: bytes, signature: str) -> bool:
    """La firma que manda Bubble: HMAC del cuerpo CRUDO, no del JSON reescrito."""
    want = "sha256=" + hmac.new(SECRET.encode(), body, hashlib.sha256).hexdigest()
    return bool(SECRET) and hmac.compare_digest(want.encode(), signature.encode())


LAMP = {"green": "\033[42m  \033[0m", "yellow": "\033[43m  \033[0m", "red": "\033[41m  \033[0m"}


def draw():
    if LOG or HEADLESS:
        return
    print("\033[2J\033[H", end="")
    print(f"Semáforos simulados · escuchando en {HOST}:{PORT} · {time.strftime('%H:%M:%S')}\n")
    for light in sorted(lights.values(), key=lambda x: (-x["level"], x["name"])):
        lamp = LAMP.get(light["color"], "  ")
        if light["blink"] and int(time.time()) % 2:
            lamp = "  "
        why = f"  · {light['reasons'][0]}" if light["reasons"] else ""
        print(f"  {lamp}  {light['mood']:<9} {light['name']:<24}{why}")
    if not lights:
        print("  (todavía nada: en Bubble, Ajustes → Furia → «Enviar latido ahora»)")


def valid_light(light):
    """Validación mínima para que un POST mal formado no tumbe el simulador."""
    if not isinstance(light, dict):
        return False
    if any(not isinstance(light.get(key), str) for key in ("person", "name", "mood", "at")):
        return False
    return (bool(light["person"])
            and light.get("type") in ("fury.changed", "fury.heartbeat")
            and type(light.get("level")) is int and light["level"] in range(4)
            and light.get("color") in LAMP
            and isinstance(light.get("blink"), bool)
            and isinstance(light.get("reasons"), list)
            and all(isinstance(reason, str) for reason in light["reasons"]))


class Handler(BaseHTTPRequestHandler):
    def respond(self, status, body=None):
        data = json.dumps(body, ensure_ascii=False).encode() if body is not None else b""
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path == "/health":
            return self.respond(200, {"ok": True, "mode": "simulated-gateway"})
        if self.path == "/state":
            expected = "Bearer " + VIEW_TOKEN
            got = self.headers.get("Authorization", "")
            if not VIEW_TOKEN or not hmac.compare_digest(got.encode(), expected.encode()):
                return self.respond(401, {"error": "unauthorized"})
            return self.respond(200, {"mode": "simulated-gateway", "lights": sorted(
                lights.values(), key=lambda light: (-light["level"], light["person"]))})
        self.respond(404, {"error": "not found"})

    def do_POST(self):
        if self.path != "/fury":
            return self.respond(404, {"error": "not found"})
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            return self.respond(400, {"error": "invalid content length"})
        if not 0 < length <= MAX_BODY:
            return self.respond(413, {"error": "invalid body size"})
        self.connection.settimeout(5)
        body = self.rfile.read(length)
        ok = valid(body, self.headers.get("X-Bubble-Signature", ""))
        try:
            light = json.loads(body)
        except (ValueError, UnicodeError):
            light = None
        if LOG:
            with open(LOG, "a") as log:
                log.write(json.dumps({"valid": ok, "event": self.headers.get("X-Bubble-Event", ""),
                                      "body": light if isinstance(light, dict) else {}}) + "\n")
        if not ok:
            return self.respond(401, {"error": "invalid signature"})
        if not valid_light(light):
            return self.respond(400, {"error": "invalid fury event"})
        lights[light["person"]] = light
        self.respond(204)
        if HEADLESS:
            print(json.dumps({"event": light["type"], "person": light["person"],
                              "level": light["level"], "mood": light["mood"]}), flush=True)
        draw()

    def log_message(self, *args):
        pass


def main():
    global LOG, PORT
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("port", type=int, nargs="?", default=8787)
    parser.add_argument("--log", default="")
    args = parser.parse_args()
    PORT, LOG = args.port, args.log
    if not SECRET:
        sys.exit("fury listen: falta BUBBLE_FURY_SECRET (el secreto que enseña Ajustes → Furia una vez)")
    draw()
    HTTPServer((HOST, PORT), Handler).serve_forever()


if __name__ == "__main__":
    main()
