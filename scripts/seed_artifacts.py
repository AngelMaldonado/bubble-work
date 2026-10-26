#!/usr/bin/env python3
"""Artefactos temporales de ejemplo: un interrogatorio y un markdown efímero.

    just seed artifacts

Los crea por MCP, como un agente, a nombre de la persona lead de .env: un
interrogatorio «Demo: …» con preguntas en dos rondas (una contestada, otra en
la frontera) y un markdown «Demo: …» con un documento. Idempotente: si ya hay
uno con ese título abierto, no crea otro. Caducan solos como cualquier
artefacto (30 días por omisión). Sólo contra localhost.
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
if urllib.parse.urlparse(API).hostname not in ("127.0.0.1", "localhost", "::1"):
    sys.exit(f"seed: sólo contra el servidor local, no {API}")
if not EMAIL or not PASSWORD:
    sys.exit("seed: faltan BUBBLE_DEV_SUPERUSER_EMAIL / _PASSWORD en .env")

GRILL = "Demo: ¿qué hacemos con el onboarding?"
MD = "Demo: propuesta de checkout en un paso"


def post(path, body, token=""):
    req = urllib.request.Request(API + path, data=json.dumps(body).encode(), method="POST")
    req.add_header("Content-Type", "application/json")
    req.add_header("Accept", "application/json, text/event-stream")
    if token:
        req.add_header("Authorization", token)
    try:
        with urllib.request.urlopen(req) as res:
            return res.read().decode()
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"POST {path} -> {e.code}: {e.read().decode(errors='replace')[:300]}") from None
    except urllib.error.URLError as e:
        sys.exit(f"seed: no responde {API} ({e.reason}). ¿Está corriendo `just dev`?")


TOKEN = json.loads(post("/api/collections/users/auth-with-password", {"identity": EMAIL, "password": PASSWORD}))["token"]


def tool(name, args):
    raw = post("/mcp", {"jsonrpc": "2.0", "id": 1, "method": "tools/call",
                        "params": {"name": name, "arguments": args}}, TOKEN)
    d = json.loads((re.findall(r"^data: (.*)$", raw, re.M) or [raw])[-1])
    r = d.get("result") or {}
    text = (r.get("content") or [{}])[0].get("text", "")
    if r.get("isError"):
        raise RuntimeError(f"{name}: {text[:300]}")
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        # artifact_create trae el protocolo en un segundo bloque: el primero es el JSON
        return json.loads(text.split("\n#", 1)[0]) if text.startswith("{") else text


def q(qid, rnd, title, body, choices, rec, deps=()):
    return {"type": "question", "data": {"id": qid, "round": rnd, "title": title, "body": body,
                                         "choices": choices, "recommendation": rec, "depends_on": list(deps)}}


have = {a["title"]: a["id"] for a in (tool("artifact_list", {}) or []) if a.get("status") == "open"}
made = 0

if GRILL not in have:
    g = tool("artifact_create", {"kind": "grill", "topic": GRILL})
    tool("artifact_append", {"id": g["id"], "entries": [
        q("Q1", 1, "¿Para quién es?", "¿El onboarding nuevo es para clientes nuevos o también para los que ya están?",
          ["(a) Sólo nuevos", "(b) Todos"], "(a): los que ya están ya aprendieron; cambiarles el flujo cuesta soporte."),
        q("Q2", 1, "¿Cuánto mide?", "¿Cuántos pasos como máximo antes de llegar al tablero?",
          ["(a) 3", "(b) 5"], "(a) 3: cada paso extra pierde gente."),
        {"type": "answer", "data": {"question": "Q1", "verdict": "accepted", "text": ""}},
        q("Q3", 2, "¿Se puede saltar?", "Si alguien ya sabe usarlo, ¿lo dejamos saltar el onboarding?",
          ["(a) Sí", "(b) No"], "(a): obligar a quien ya sabe es la forma más rápida de que lo odie.", ["Q1"]),
    ]})
    made += 1

# Un markdown sin documento (un intento que se quedó a medias) se completa.
md_id = have.get(MD)
if md_id and (tool("artifact_read", {"id": md_id}) or {}).get("document"):
    md_id = "done"
if md_id != "done":
    if not md_id:
        md_id = tool("artifact_create", {"kind": "markdown", "topic": MD})["id"]
    tool("artifact_append", {"id": md_id, "entries": [{"type": "document", "data": {
        "filename": "checkout.md", "base": 0,
        "markdown": "# Checkout en un paso\n\nHoy son cuatro pantallas; la propuesta es una.\n\n"
                    "## Qué cambia\n\n- Dirección, pago y confirmación en la misma vista\n"
                    "- Tarjeta guardada por omisión\n\n## Qué no\n\n- [ ] El cálculo de envío\n"
                    "- [ ] Los cupones\n\n```mermaid\nflowchart LR\n  Carrito --> Pago --> Listo\n```\n",
    }}]})
    made += 1

print(f"seed artifacts: {made} creados ({2 - made} ya estaban) · ábrelos en el cerebro o en el inbox")
