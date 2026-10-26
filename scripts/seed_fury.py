#!/usr/bin/env python3
"""Hilos y burbujas en cada nivel de furia de IA, para ver cómo se ve.

    just seed fury

Un proyecto «Demo Furia» en el equipo «Demo Producto», y ENCIENDE la furia en
el dev (`features.fury`). Con las reglas de serie deja:

  🔥 Desatada  «Pago a proveedores»      P1, vencido hace 14 días
  😡 Furiosa   «Conciliación bancaria»   P2, vencido hace 2 días
  😤 Irritada  «Alta de clientes»        P1 sin asignados
  😌 Zen       «Reporte mensual»         P3 vencido: no es prioridad alta
  burbuja «Lanzamiento» (impacto y urgencia altos, vencida hace 3 días): 😡
      y además toma el 🔥 de su hilo
  burbuja «Sin responsable» (impacto y urgencia altos, nadie a cargo, sin
      hilos): 😡 — vacía y sin responsable, el calor la pone en la tumba, así
      que dispara «sin responsables» y también «olvidada»

Sólo las reglas que dependen de datos (prioridad, plazo, asignados): «olvidada»
y «burbuja inflada» necesitan que pase el tiempo, y como los demás seeds éste no
falsea fechas para no mentirle al tablero.

Los plazos se ponen relativos a HOY cada vez que corre: es idempotente y deja
los niveles como dice arriba. Escribe por la API del dev como la persona lead
de .env; se niega a correr contra algo que no sea localhost.
"""
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timedelta, timezone

HTTP = os.environ.get("BUBBLE_HTTP", "127.0.0.1:8090")
API = "http://" + HTTP.replace("0.0.0.0", "127.0.0.1")
EMAIL = os.environ.get("BUBBLE_DEV_SUPERUSER_EMAIL", "")
PASSWORD = os.environ.get("BUBBLE_DEV_SUPERUSER_PASSWORD", "")
if urllib.parse.urlparse(API).hostname not in ("127.0.0.1", "localhost", "::1"):
    sys.exit(f"seed: sólo contra el servidor local, no {API}")
if not EMAIL or not PASSWORD:
    sys.exit("seed: faltan BUBBLE_DEV_SUPERUSER_EMAIL / _PASSWORD en .env")

TOKEN = ""


def call(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(API + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if TOKEN:
        req.add_header("Authorization", TOKEN)
    try:
        with urllib.request.urlopen(req) as res:
            raw = res.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{method} {path} -> {e.code}: {e.read().decode(errors='replace')[:300]}") from None
    except urllib.error.URLError as e:
        sys.exit(f"seed: no responde {API} ({e.reason}). ¿Está corriendo `just dev`?")


def find(collection, filt):
    q = urllib.parse.urlencode({"filter": filt, "perPage": 1})
    items = call("GET", f"/api/collections/{collection}/records?{q}").get("items", [])
    return items[0] if items else None


def esc(s):
    return s.replace("\\", "\\\\").replace('"', '\\"')


def day(offset):
    d = datetime.now(timezone.utc).date() + timedelta(days=offset)
    return f"{d.isoformat()} 00:00:00.000Z"


def upsert(collection, filt, create, update):
    """Crea si falta; si existe, deja `update` como debe ser."""
    got = find(collection, filt)
    if not got:
        return call("POST", f"/api/collections/{collection}/records", {**create, **update}), True
    if any(str(got.get(k) or "") != str(v or "") for k, v in update.items() if not isinstance(v, list)) or \
            any(sorted(got.get(k) or []) != sorted(v) for k, v in update.items() if isinstance(v, list)):
        got = call("PATCH", f"/api/collections/{collection}/records/{got['id']}", update)
    return got, False


def main():
    global TOKEN
    auth = call("POST", "/api/collections/users/auth-with-password", {"identity": EMAIL, "password": PASSWORD})
    TOKEN = auth["token"]
    me = auth["record"]
    if me.get("role") != "lead":
        sys.exit("seed: la persona de .env tiene que ser lead global")

    teams = {t["name"]: t["id"] for t in call("GET", "/api/teams")["teams"]}
    team = teams.get("Demo Producto") or call("POST", "/api/teams", {"name": "Demo Producto"})["id"]
    ws, _ = upsert("workspaces", 'slug = "demo-furia"', {"slug": "demo-furia", "team": team}, {"name": "Demo Furia"})

    def bubble(name, update):
        b, _ = upsert("bubbles", f'workspace = "{ws["id"]}" && name = "{esc(name)}"',
                      {"workspace": ws["id"], "name": name}, update)
        return b["id"]

    launch = bubble("Lanzamiento", {"outcome": "El producto está en manos de los primeros clientes.",
                                    "impact": "high", "urgency": "high", "owners": [me["id"]], "due_date": day(-3)})
    bubble("Sin responsable", {"outcome": "Alguien responde por esto.",
                               "impact": "high", "urgency": "high", "owners": [], "due_date": ""})

    def thread(name, update):
        t, _ = upsert("threads", f'workspace = "{ws["id"]}" && name = "{esc(name)}"',
                      {"workspace": ws["id"], "name": name}, update)
        return t["id"]

    thread("Pago a proveedores", {"bubble": launch, "priority": "P1", "due_date": day(-14), "assignees": [me["id"]]})
    thread("Conciliación bancaria", {"bubble": launch, "priority": "P2", "due_date": day(-2), "assignees": [me["id"]]})
    thread("Alta de clientes", {"bubble": launch, "priority": "P1", "due_date": "", "assignees": []})
    thread("Reporte mensual", {"bubble": launch, "priority": "P3", "due_date": day(-5), "assignees": [me["id"]]})

    feats = call("GET", "/api/collections/features/records?perPage=1")["items"][0]
    if not feats.get("fury"):
        call("PATCH", f"/api/collections/features/records/{feats['id']}", {"fury": True})

    board = call("GET", f"/api/workspaces/{ws['id']}/board")
    names = {0: "😌", 1: "😤", 2: "😡", 3: "🔥"}
    rows = [(x.get("fury") or {}).get("level", 0) for x in board["bubbles"] + board["threads"]]
    print("seed fury: «Demo Furia» con", ", ".join(f"{rows.count(n)} {names[n]}" for n in (3, 2, 1, 0)),
          "· furia encendida en el dev")


main()
