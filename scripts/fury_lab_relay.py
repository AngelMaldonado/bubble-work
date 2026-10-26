#!/usr/bin/env python3
"""Puente SÓLO del lab: localhost → curl del sistema → HTTPS en reko-3.

Python/Go en este Mac no alcanzan la LAN (errno 65), aunque curl sí. No toca
permisos, no cambia el contrato ni firma en nombre de Bubble: pasa los mismos
bytes y la misma firma al gateway, que la verifica. No es un proxy genérico.
El gateway real de la oficina no necesita este puente.
"""
from http.server import BaseHTTPRequestHandler, HTTPServer
import subprocess

URL = "https://fury-gateway.cubytest.space/fury"


class Relay(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/fury":
            self.send_error(404)
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            self.send_error(400)
            return
        if not 0 < length <= 65536:
            self.send_error(413)
            return
        self.connection.settimeout(5)
        body = self.rfile.read(length)
        # No secretos por argv: la firma es la del aviso (no la clave HMAC).
        command = ["curl", "--silent", "--show-error", "--connect-timeout", "2", "--max-time", "4",
                   "--output", "/dev/null", "--write-out", "%{http_code}", "--request", "POST",
                   "--header", "Content-Type: application/json", "--header",
                   "X-Bubble-Signature: " + self.headers.get("X-Bubble-Signature", ""),
                   "--header", "X-Bubble-Event: " + self.headers.get("X-Bubble-Event", ""),
                   "--data-binary", "@-", URL]
        try:
            result = subprocess.run(command, input=body, stdout=subprocess.PIPE, timeout=5)
            status = int(result.stdout) if result.returncode == 0 else 502
        except (subprocess.TimeoutExpired, ValueError):
            status = 502
        self.send_response(status)
        self.send_header("Content-Length", "0")
        self.end_headers()
        print(f"relay → gateway HTTPS: {status}", flush=True)

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    print("Puente temporal del lab: 127.0.0.1:18787 → gateway HTTPS (misma firma)", flush=True)
    HTTPServer(("127.0.0.1", 18787), Relay).serve_forever()
