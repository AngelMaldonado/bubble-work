#!/usr/bin/env python3
"""Prueba DEV → HTTPS/Traefik de reko-3 → gateway SIMULADO, sin hardware.

just fury lab up      construye, configura SÓLO el webhook del dev y envía un latido
just fury lab relay   puente opcional si el Mac impide al dev llegar a la LAN
just fury lab test    envía un latido del dev al contenedor
just fury lab state   muestra las luces recibidas (GET autenticado, sin imprimir tokens)
just fury lab logs    sigue los eventos del receptor
just fury lab status  estado del contenedor
just fury lab stop    desmonta sólo este lab y desconecta su webhook del dev

Nunca acepta un origen Bubble que no sea loopback. Usa las credenciales DEV
existentes de .env, y guarda sólo secretos del lab en deploy/fury-lab/.env
(gitignored, modo 600). No cambia DNS, certificados ni routers ajenos.
"""
import json
import os
from pathlib import Path
import secrets
import shlex
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
LAB = ROOT / "deploy/fury-lab"
ENV = LAB / ".env"
URL = "https://fury-gateway.cubytest.space"
HOOK = URL + "/fury"
RELAY = "http://127.0.0.1:18787/fury"
SESSION = "bubble-work-fury-lab"
DOCKER = ["docker", "--context", "reko-3", "compose", "-f", str(LAB / "compose.yaml")]
TOKEN = ""
API = "http://" + os.environ.get("BUBBLE_HTTP", "127.0.0.1:8090").replace("0.0.0.0", "127.0.0.1")


def request(url, method="GET", body=None, token=""):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers={"Content-Type": "application/json"})
    if token:
        req.add_header("Authorization", token)
    with urllib.request.urlopen(req, timeout=30) as response:
        data = response.read()
        return json.loads(data) if data else {}


def api(path, method="GET", body=None):
    return request(API + path, method, body, TOKEN)


def login():
    global TOKEN
    if urllib.parse.urlparse(API).hostname not in ("localhost", "127.0.0.1", "::1"):
        sys.exit("fury lab: sólo se puede conectar al Bubble local de desarrollo")
    email = os.environ.get("BUBBLE_DEV_SUPERUSER_EMAIL", "")
    password = os.environ.get("BUBBLE_DEV_SUPERUSER_PASSWORD", "")
    if not email or not password:
        sys.exit("fury lab: faltan las credenciales DEV en .env")
    auth = api("/api/collections/users/auth-with-password", "POST", {"identity": email, "password": password})
    if auth["record"].get("role") != "lead":
        sys.exit("fury lab: la persona DEV de .env tiene que ser lead global")
    TOKEN = auth["token"]


def compose(*args):
    subprocess.run(DOCKER + list(args), cwd=ROOT, check=True)


def env():
    if not ENV.exists():
        sys.exit("fury lab: primero ejecuta `just fury lab up`")
    return dict(line.split("=", 1) for line in ENV.read_text().splitlines() if "=" in line)


def gateway(path, token=""):
    # curl del sistema sí llega a la LAN en este Mac; Python puede no tener
    # permiso de red local. Sin -k: se comprueban certificado y hostname.
    # La cabecera secreta viaja por stdin, nunca por argv ni por la consola.
    config = "url = " + json.dumps(URL + path) + "\n"
    if token:
        config += "header = " + json.dumps("Authorization: Bearer " + token) + "\n"
    result = subprocess.run(
        ["curl", "--silent", "--show-error", "--fail", "--connect-timeout", "5",
         "--max-time", "15", "--config", "-"],
        input=config, text=True, stdout=subprocess.PIPE, check=True)
    return json.loads(result.stdout)


def state():
    result = gateway("/state", token=env()["BUBBLE_FURY_VIEW_TOKEN"])
    print("Gateway SIMULADO · " + URL)
    print("Sin radio ni watchdog; estas son las últimas luces recibidas, no hardware conectado.")
    for light in result["lights"]:
        lamp = {"green": "🟢", "yellow": "🟡", "red": "🔴"}[light["color"]]
        blink = " (parpadeo)" if light["blink"] else ""
        print(f"{lamp} {light['mood']:<9} {light['name']}{blink} · {light['type']} · {light['at']}")
    print(f"{len(result['lights'])} personas")


def heartbeat():
    login()
    if api("/api/fury/hook")["url"] not in (HOOK, RELAY):
        sys.exit("fury lab: el webhook del dev no apunta al lab; usa `just fury lab up`")
    result = api("/api/fury/hook/test", "POST", {})
    print(result["status"])
    if result["failed"] or not result["sent"]:
        sys.exit("fury lab: el latido falló o no envió personas")


def main():
    action = sys.argv[1] if len(sys.argv) == 2 else ""
    if action == "up":
        login()
        previous = api("/api/fury/hook")
        if previous["url"] not in ("", HOOK, RELAY):
            sys.exit("fury lab: el dev ya tiene otro receptor; no lo sobrescribo")
        if not ENV.exists():
            fresh = api("/api/fury/hook", "POST", {"rotate": True})["secret"]
            content = f"BUBBLE_FURY_SECRET={fresh}\nBUBBLE_FURY_VIEW_TOKEN={secrets.token_hex(24)}\n"
            fd = os.open(ENV, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, "w") as file:
                file.write(content)
        compose("up", "-d", "--build", "--wait", "--wait-timeout", "60")
        for attempt in range(30):
            try:
                health = gateway("/health")
                if health.get("mode") == "simulated-gateway":
                    break
            except (OSError, ValueError, subprocess.CalledProcessError):
                pass
            time.sleep(1)
        else:
            sys.exit("fury lab: no responde por HTTPS; revisa `just fury lab logs`")
        # Conserva el puente opcional si ya fue elegido para este Mac.
        api("/api/fury/hook", "POST", {"url": previous["url"] or HOOK})
        heartbeat()
        state()
    elif action == "relay":
        login()
        if api("/api/fury/hook")["url"] not in ("", HOOK, RELAY):
            sys.exit("fury lab: el dev tiene otro receptor; no lo sobrescribo")
        env()  # exige un lab ya desplegado
        exists = subprocess.run(["tmux", "has-session", "-t", SESSION], stderr=subprocess.DEVNULL).returncode == 0
        if not exists:
            command = shlex.join([sys.executable, "-u", str(ROOT / "scripts/fury_lab_relay.py")])
            subprocess.run(["tmux", "new-session", "-d", "-s", SESSION, "-n", "relay", "-c", str(ROOT), command], check=True)
            subprocess.run(["tmux", "set-option", "-t", SESSION, "@owner", "agent"], check=True)
            subprocess.run(["tmux", "set-option", "-t", SESSION, "@purpose", "Puente temporal del dev hacia el gateway HTTPS de reko-3"], check=True)
            time.sleep(0.5)
        api("/api/fury/hook", "POST", {"url": RELAY})
        print("Puente temporal: Bubble DEV → localhost → curl → gateway HTTPS; cuerpo y firma intactos.")
        heartbeat()
        state()
    elif action == "test":
        heartbeat()
        state()
    elif action == "state":
        state()
    elif action == "status":
        compose("ps")
    elif action == "logs":
        compose("logs", "--tail", "30", "-f")
    elif action == "stop":
        # Desconectar el emisor antes de quitar el receptor; nunca otro destino.
        try:
            login()
            if api("/api/fury/hook")["url"] in (HOOK, RELAY):
                api("/api/fury/hook", "POST", {"url": ""})
        except urllib.error.URLError:
            print("Dev no disponible; al volver, desconecta su webhook si todavía apunta al lab.")
        compose("down")
        pane = subprocess.run(["tmux", "display-message", "-p", "-t", SESSION,
                               "#{session_attached} #{@owner}"], capture_output=True, text=True)
        if pane.returncode == 0 and pane.stdout.strip() == "0 agent":
            subprocess.run(["tmux", "kill-session", "-t", SESSION], check=True)
        elif pane.returncode == 0:
            print("Puente adjunto o ajeno: no se cerró la sesión " + SESSION)
    else:
        sys.exit("usage: just fury lab up|relay|test|state|logs|status|stop")


if __name__ == "__main__":
    try:
        main()
    except (urllib.error.URLError, ValueError, subprocess.CalledProcessError) as error:
        sys.exit(f"fury lab: {error}")
