#!/usr/bin/env python3
"""Datos de ejemplo para ver la vista Blobs con algo parecido a la realidad.

    just seed tags

Escribe por la API REST del servidor de DESARROLLO (BUBBLE_HTTP, por defecto
127.0.0.1:8090) como la persona lead de .env (BUBBLE_DEV_SUPERUSER_*). Se niega
a correr contra otra cosa que no sea localhost.

Todo lo que crea lleva «Demo» en el nombre del equipo o del proyecto, para que
no se confunda con trabajo real. Es idempotente: busca antes de crear y sólo
reescribe un documento si sus tags no son los esperados.

Lo que deja:
  - dos equipos compartidos, «Demo Producto» y «Demo Operaciones», con dos
    proyectos cada uno;
  - burbujas con hilos cuyo documento lleva frontmatter `tags: [...]`: el
    primero decide el blob (pagos, onboarding, infra…), el segundo dice
    la superficie (móvil, web…); algunos tags se repiten entre equipos («calidad», «pagos») para
    ver que son el mismo blob;
  - una burbuja cerrada, hilos sueltos, hilos sin tags, y dependencias y
    relaciones entre hilos del mismo equipo (nunca entre equipos: esas no son
    aristas).

No inventa calor: escribir el documento es una escritura real y calienta como
cualquier otra; las burbujas sin hilos salen dormidas.
"""
import json
import os
from datetime import datetime, timezone
import sys
import urllib.error
import urllib.parse
import urllib.request

HTTP = os.environ.get("BUBBLE_HTTP", "127.0.0.1:8090")
API = "http://" + HTTP.replace("0.0.0.0", "127.0.0.1")
EMAIL = os.environ.get("BUBBLE_DEV_SUPERUSER_EMAIL", "")
PASSWORD = os.environ.get("BUBBLE_DEV_SUPERUSER_PASSWORD", "")

host = urllib.parse.urlparse(API).hostname
if host not in ("127.0.0.1", "localhost", "::1"):
    sys.exit(f"seed: sólo contra el servidor local, no {API}")
if not EMAIL or not PASSWORD:
    sys.exit("seed: faltan BUBBLE_DEV_SUPERUSER_EMAIL / _PASSWORD en .env (la persona lead del dev)")

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
        detail = e.read().decode(errors="replace")
        raise RuntimeError(f"{method} {path} -> {e.code}: {detail}") from None
    except urllib.error.URLError as e:
        sys.exit(f"seed: no responde {API} ({e.reason}). ¿Está corriendo `just dev`?")


def find(collection, filt):
    q = urllib.parse.urlencode({"filter": filt, "perPage": 1})
    items = call("GET", f"/api/collections/{collection}/records?{q}").get("items", [])
    return items[0] if items else None


def esc(s):
    return s.replace("\\", "\\\\").replace('"', '\\"')


def ensure(collection, filt, fields):
    got = find(collection, filt)
    if got:
        return got, False
    return call("POST", f"/api/collections/{collection}/records", fields), True


# ---- lo que se siembra --------------------------------------------------------

TEAMS = {
    "Demo Producto": {
        "projects": {
            "demo-app-clientes": ("Demo App Clientes", [
                ("Checkout en un paso", "Pagos", True, ["Diseñar el flujo", "Integrar tarjetas guardadas", "Pruebas A/B del botón"]),
                ("Registro sin contraseña", "Onboarding", True, ["Enlace mágico por correo", "Verificar teléfono"]),
                ("Modo sin conexión", "Experiencia móvil", True, ["Cola de acciones offline", "Resolver conflictos al reconectar"]),
                ("Notificaciones push", "Experiencia móvil", False, ["Permisos en iOS", "Centro de preferencias"]),
                ("Pruebas de regresión visual", "Calidad", True, ["Capturas por pantalla"]),
                ("Accesibilidad AA", "Clientes", False, []),
            ]),
            "demo-portal-web": ("Demo Portal Web", [
                ("Facturación en línea", "Pagos", True, ["Descargar CFDI", "Historial de pagos"]),
                ("Panel de uso", "Datos", True, ["Métricas de consumo", "Exportar a CSV"]),
                ("Bienvenida guiada", "Onboarding", False, ["Tour de la primera sesión"]),
                ("Inicio de sesión con SSO", "Seguridad", True, ["SAML con Azure AD"]),
                ("Rediseño del menú", None, False, []),
            ]),
        },
        "links": [
            ("Diseñar el flujo", "blocking", "Integrar tarjetas guardadas"),
            ("Integrar tarjetas guardadas", "blocking", "Historial de pagos"),
            ("Enlace mágico por correo", "blocking", "Tour de la primera sesión"),
            ("Cola de acciones offline", "blocking", "Resolver conflictos al reconectar"),
            ("Métricas de consumo", "relates_to", "Pruebas A/B del botón"),
            ("SAML con Azure AD", "relates_to", "Enlace mágico por correo"),
        ],
        "loose": {"demo-app-clientes": ["Revisar reseñas de la tienda"], "demo-portal-web": ["Actualizar dependencias"]},
        "closed": ("demo-portal-web", "Rediseño del menú"),
    },
    "Demo Operaciones": {
        "projects": {
            "demo-plataforma": ("Demo Plataforma", [
                ("Migrar a Postgres 17", "Infraestructura", True, ["Ensayo en staging", "Ventana de mantenimiento", "Retirar réplicas viejas"]),
                ("Respaldos verificados", "Infraestructura", True, ["Restaurar semanal automático"]),
                ("Rotación de secretos", "Seguridad", True, ["Inventario de llaves", "Rotar llaves de pago"]),
                ("Lago de datos", "Datos", False, ["Ingesta diaria", "Catálogo de tablas"]),
                ("Costos de nube", "Infraestructura", False, []),
            ]),
            "demo-soporte": ("Demo Soporte", [
                ("Mesa de ayuda nivel 1", "Soporte", True, ["Macros de respuesta", "Encuesta de satisfacción"]),
                ("Reembolsos", "Pagos", True, ["Política de reembolsos", "Automatizar reembolsos parciales"]),
                ("Base de conocimiento", "Clientes", False, ["Artículos de facturación"]),
                ("Auditoría de calidad", "Calidad", True, ["Muestreo de tickets"]),
            ]),
        },
        "links": [
            ("Ensayo en staging", "blocking", "Ventana de mantenimiento"),
            ("Ventana de mantenimiento", "blocking", "Retirar réplicas viejas"),
            ("Inventario de llaves", "blocking", "Rotar llaves de pago"),
            ("Rotar llaves de pago", "blocking", "Automatizar reembolsos parciales"),
            ("Ingesta diaria", "relates_to", "Encuesta de satisfacción"),
            ("Política de reembolsos", "relates_to", "Artículos de facturación"),
        ],
        "loose": {"demo-soporte": ["Revisar guardias de diciembre"]},
        "closed": None,
    },
}


# El segundo tag de cada hilo: de qué superficie es.
AREA = {"demo-app-clientes": "móvil", "demo-portal-web": "web", "demo-plataforma": "plataforma", "demo-soporte": "soporte"}


def document(name, tags):
    front = f"---\ntags: [{', '.join(tags)}]\n---\n" if tags else ""
    return f"{front}# {name}\n\nEjemplo sembrado por `just seed tags`.\n\n- [ ] Siguiente paso\n"


def write_tags(thread_id, name, tags):
    """Reescribe el documento sólo si sus tags no son los esperados."""
    doc = call("GET", f"/api/threads/{thread_id}/document")
    t = call("GET", f"/api/collections/threads/records/{thread_id}")
    if (t.get("tags") or []) == tags and doc.get("content"):
        return False
    call("PATCH", f"/api/threads/{thread_id}/document", {"base": doc["hash"], "content": document(name, tags)})
    return True


def main():
    global TOKEN
    auth = call("POST", "/api/collections/users/auth-with-password", {"identity": EMAIL, "password": PASSWORD})
    TOKEN = auth["token"]
    me = auth["record"]
    if me.get("role") != "lead":
        sys.exit("seed: la persona de .env tiene que ser lead global (crea equipos)")
    made = {"equipos": 0, "proyectos": 0, "burbujas": 0, "hilos": 0, "documentos": 0, "relaciones": 0}

    teams = {t["name"]: t["id"] for t in call("GET", "/api/teams")["teams"]}
    for team_name, spec in TEAMS.items():
        if team_name not in teams:
            teams[team_name] = call("POST", "/api/teams", {"name": team_name})["id"]
            made["equipos"] += 1
        team = teams[team_name]
        threads = {}
        workspaces = {}
        for slug, (ws_name, bubbles) in spec["projects"].items():
            ws, new = ensure("workspaces", f'slug = "{slug}"', {"name": ws_name, "slug": slug, "team": team})
            workspaces[slug] = ws["id"]
            made["proyectos"] += new
            for b_name, topic, owned, thread_names in bubbles:
                b, new = ensure("bubbles", f'workspace = "{ws["id"]}" && name = "{esc(b_name)}"', {
                    "workspace": ws["id"], "name": b_name,
                    "outcome": f"{b_name}: listo y en manos de quien lo usa.",
                    "owners": [me["id"]] if owned else [],
                })
                made["burbujas"] += new
                # Sin repetir, como los normaliza el servidor («soporte» en Demo Soporte).
                tags = list(dict.fromkeys([topic.lower(), AREA[slug]])) if topic else []
                for t_name in thread_names:
                    t, new = ensure("threads", f'workspace = "{ws["id"]}" && name = "{esc(t_name)}"',
                                    {"workspace": ws["id"], "bubble": b["id"], "name": t_name})
                    threads[t_name] = t["id"]
                    made["hilos"] += new
                    made["documentos"] += write_tags(t["id"], t_name, tags)
        for slug, names in spec["loose"].items():
            for t_name in names:
                t, new = ensure("threads", f'workspace = "{workspaces[slug]}" && name = "{esc(t_name)}"',
                                {"workspace": workspaces[slug], "name": t_name})
                threads[t_name] = t["id"]
                made["hilos"] += new
        for a, kind, b in spec["links"]:
            _, new = ensure("thread_relations", f'thread = "{threads[a]}" && related = "{threads[b]}"',
                            {"thread": threads[a], "related": threads[b], "type": kind})
            made["relaciones"] += new
        if spec["closed"]:
            slug, b_name = spec["closed"]
            b = find("bubbles", f'workspace = "{workspaces[slug]}" && name = "{esc(b_name)}"')
            if b and not b.get("closed_at"):
                call("PATCH", f"/api/collections/bubbles/records/{b['id']}",
                     {"closed_at": datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S.000Z"),
                      "closure": "Se decidió no rediseñarlo este trimestre."})

    print("seed tags:", ", ".join(f"{v} {k}" for k, v in made.items()), "nuevos o reescritos")
    print(f"ábrelo en Todos los equipos → Blobs, o en un proyecto «Demo …» ({API})")


if __name__ == "__main__":
    main()
