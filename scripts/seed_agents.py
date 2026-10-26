#!/usr/bin/env python3
"""Personas y agentes simulados en el dev, para ver Ajustes → Usuarios y Tokens.

    just seed agents

Todas llevan un correo `@simulado.test`, y se borran solas al bajar el dev
(`just stop`, `just dev`, Ctrl-C o una caída: `bubble simulated purge`). Correrlo
otra vez regenera sus tokens; no duplica personas.

Cada agente hace los handshakes MCP de verdad —`initialize` con su
`clientInfo` y la cabecera `X-Bubble-Connect`, y luego `agents_md`— en el orden
que deja a cada uno en un estado: al día, release anterior, nunca leyó,
instalación vieja, sin usar, revocado.

Para que haya una release «anterior», el lead simulado vuelve a publicar el
AGENTS.md de operadores con el MISMO texto que ya tenía más un salto de línea:
los agentes reales del dev siguen leyendo lo mismo, firmado por «Lead simulado»
hasta que se limpie (la limpieza borra sus versiones y vuelve la tuya).

Escribe por la API del dev (BUBBLE_HTTP) con la persona y el superuser de .env.
Se niega a correr contra algo que no sea localhost.
"""
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

HTTP = os.environ.get("BUBBLE_HTTP", "127.0.0.1:8090")
API = "http://" + HTTP.replace("0.0.0.0", "127.0.0.1")
EMAIL = os.environ.get("BUBBLE_DEV_SUPERUSER_EMAIL", "")
PASSWORD = os.environ.get("BUBBLE_DEV_SUPERUSER_PASSWORD", "")
DOMAIN = "@simulado.test"  # el mismo que borra `bubble simulated purge`
PW = "simulado1234"

if urllib.parse.urlparse(API).hostname not in ("127.0.0.1", "localhost", "::1"):
    sys.exit(f"seed: sólo contra el servidor local, no {API}")
if not EMAIL or not PASSWORD:
    sys.exit("seed: faltan BUBBLE_DEV_SUPERUSER_EMAIL / _PASSWORD en .env")


def req(method, path, body=None, auth=None, headers=None, raw=False):
    h = {"Content-Type": "application/json", **(headers or {})}
    if auth:
        h["Authorization"] = auth
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(API + path, data=data, method=method, headers=h)
    try:
        with urllib.request.urlopen(r) as res:
            txt = res.read().decode()
            return (txt, res.headers) if raw else (json.loads(txt) if txt else {})
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{method} {path} -> {e.code}: {e.read().decode(errors='replace')[:300]}") from None
    except urllib.error.URLError as e:
        sys.exit(f"seed: no responde {API} ({e.reason}). ¿Está corriendo `just dev`?")


def login(coll, email, pw):
    return req("POST", f"/api/collections/{coll}/auth-with-password", {"identity": email, "password": pw})["token"]


SU = login("_superusers", EMAIL, PASSWORD)
ME = login("users", EMAIL, PASSWORD)
try:
    req("GET", "/api/agents/release", auth=ME)
except RuntimeError:
    sys.exit("seed: este servidor no sabe de releases; `just dev` para recompilarlo")

# (clave, nombre, rol, agentes). Un agente es (nombre del token, clientInfo,
# versión, estado). Estados: current, stale, never, oldinstall, unused, revoked.
PEOPLE = [
    ("lead", "Lead simulado", "lead", [("Claude Code", "claude-code", "2.1.304", "current")]),
    ("ana", "Ana Ruiz", "member", [("Claude Code del portátil", "claude-code", "2.1.304", "current"),
                                   ("Cursor", "cursor-vscode", "1.7.2", "current")]),
    ("luis", "Luis Pérez", "member", [("Codex", "codex-mcp-client", "0.160.0", "stale")]),
    ("marta", "Marta Gómez", "member", [("Antigravity", "antigravity", "1.11.2", "never")]),
    ("diego", "Diego Soto", "member", [("Claude Code de la torre", "claude-code", "2.1.290", "oldinstall"),
                                       ("VS Code", "Visual Studio Code", "1.105.0", "current")]),
    ("sofia", "Sofía Lara", "member", [("Claude Code (sin conectar)", "", "", "unused")]),
    ("pablo", "Pablo Ríos", "member", [("Gemini CLI", "gemini-cli-mcp-client", "0.9.0", "current")]),
    ("lucia", "Lucía Herrera", "member", [("Windsurf", "windsurf", "1.12.0", "stale"),
                                          ("Claude Code", "claude-code", "2.1.250", "current")]),
    ("tomas", "Tomás Vega", "member", [("Cline", "Cline", "3.32.0", "never")]),
    ("elena", "Elena Castro", "member", [("Zed", "Zed", "0.208.0", "current")]),
    ("raul", "Raúl Ortega", "member", [("pi", "pi", "1.3.0", "current"),
                                       ("Codex viejo", "codex-mcp-client", "0.150.0", "revoked")]),
    ("carmen", "Carmen Núñez", "member", []),  # sin agentes
    ("jorge", "Jorge Méndez", "member", [("Claude Code", "claude-code", "2.1.200", "stale")]),  # y la borra el lead
]

# ---- personas ----
auth, ids = {}, {}
for key, name, role, _ in PEOPLE:
    email = key + DOMAIN
    q = urllib.parse.urlencode({"filter": f"email='{email}'"})
    found = req("GET", f"/api/collections/users/records?{q}", auth=SU)["items"]
    if found:
        ids[key] = found[0]["id"]
        if found[0].get("deleted_at"):
            req("POST", f"/api/people/{ids[key]}/restore", {}, auth=ME)
    else:
        ids[key] = req("POST", "/api/people", {"email": email, "name": name, "password": PW, "role": role}, auth=ME)["id"]
    auth[key] = login("users", email, PW)

# ---- tokens: se regeneran ----
q = urllib.parse.urlencode({"filter": f"owner.email~'{DOMAIN}'", "perPage": 500})
for t in req("GET", f"/api/collections/agent_tokens/records?{q}", auth=SU)["items"]:
    req("DELETE", f"/api/collections/agent_tokens/records/{t['id']}", auth=SU)

_, hdrs = req("GET", "/api/connect", raw=True)
CONNECT = hdrs["X-Bubble-Connect-Version"]


def mcp(tok, method, params, connect, rid=1):
    h = {"Accept": "application/json, text/event-stream"}
    if connect:
        h["X-Bubble-Connect"] = connect
    txt, _ = req("POST", "/mcp", {"jsonrpc": "2.0", "id": rid, "method": method, "params": params},
                 auth="Bearer " + tok, headers=h, raw=True)
    return json.loads((re.findall(r"^data: (.*)$", txt, re.M) or [txt])[-1])


def session(tok, client, version, read=True, connect=CONNECT):
    mcp(tok, "initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                            "clientInfo": {"name": client, "version": version}}, connect)
    mcp(tok, "tools/list", {}, connect, 2)
    if read:
        mcp(tok, "tools/call", {"name": "agents_md", "arguments": {}}, connect, 3)


agents = []  # (persona, token, cliente, versión, estado, id)
for key, _, _, list_ in PEOPLE:
    for tname, client, version, state in list_:
        t = req("POST", "/api/tokens", {"name": tname, "days": 90}, auth=auth[key])
        agents.append((key, t["token"], client, version, state, t["id"]))

# Los «anterior» leen ANTES de que el lead vuelva a publicar.
for key, tok, client, version, state, _ in agents:
    if state == "stale":
        session(tok, client, version)

# El lead simulado publica otra vez el AGENTS.md de operadores: el mismo texto
# vigente más un salto de línea, así que la release cambia y lo que leen los
# agentes reales del dev no.
cur = req("GET", "/api/prompt?key=" + urllib.parse.quote("agents/operators"), auth=ME)
text = (cur.get("content") or "# Forma de trabajar\n").rstrip("\n") + "\n\n"
req("POST", "/api/prompt", {"key": "agents/operators", "content": text}, auth=auth["lead"])

for key, tok, client, version, state, tid in agents:
    if state == "current":
        session(tok, client, version)
    elif state == "never":
        session(tok, client, version, read=False)
    elif state == "oldinstall":
        session(tok, client, version, connect="0ld5ec710n0ld5ec")
    elif state == "revoked":
        session(tok, client, version)
        req("POST", f"/api/tokens/{tid}/revoke", {}, auth=auth[key])

# Una persona borrada (borrado lógico, como en la aplicación): sale en «ver las
# borradas».
req("POST", f"/api/people/{ids['jorge']}/delete", {}, auth=ME)

n = sum(1 for a in agents if a[4] != "unused")
print(f"simuladas: {len(PEOPLE)} personas{DOMAIN}, {len(agents)} tokens ({n} usados), sección vigente {CONNECT}")
print("se borran solas al bajar el dev; contraseña de todas: " + PW)
