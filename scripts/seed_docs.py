#!/usr/bin/env python3
"""Un volumen grande y realista de documentos con frontmatter, para la vista Blobs.

    just seed docs

Como `just seed tags`, pero en grande: tres equipos «Demo», once proyectos,
~80 burbujas y ~450 hilos. Cada hilo lleva un documento de verdad (para qué
existe, hecho cuando con casillas, notas) y un frontmatter con uno a tres tags
de un vocabulario compartido, con jerarquías (`cliente/acme`). Hay hilos sin
tags, hilos cuyo primer tag no es el de su burbuja, hilos completados, burbujas
cerradas y sin responsable, y relaciones dentro de cada equipo.

Escribe por la API REST del servidor de DESARROLLO (BUBBLE_HTTP) como la
persona lead de .env, y se niega a correr contra algo que no sea localhost.
Determinista (semilla fija) e idempotente: correrlo otra vez no crea ni
reescribe nada que ya esté como debe.

No inventa calor: lo recién escrito está caliente y lo completado, cerrado. Un
hilo dormido o en tumba necesita que pase el tiempo; falsear fechas sería
mentirle al tablero.
"""
import json
import os
import random
import struct
import zlib
import hashlib
import uuid
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone

HTTP = os.environ.get("BUBBLE_HTTP", "127.0.0.1:8090")
API = "http://" + HTTP.replace("0.0.0.0", "127.0.0.1")
EMAIL = os.environ.get("BUBBLE_DEV_SUPERUSER_EMAIL", "")
PASSWORD = os.environ.get("BUBBLE_DEV_SUPERUSER_PASSWORD", "")
if urllib.parse.urlparse(API).hostname not in ("127.0.0.1", "localhost", "::1"):
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
        raise RuntimeError(f"{method} {path} -> {e.code}: {e.read().decode(errors='replace')}") from None
    except urllib.error.URLError as e:
        sys.exit(f"seed: no responde {API} ({e.reason}). ¿Está corriendo `just dev`?")


def esc(s):
    return s.replace("\\", "\\\\").replace('"', '\\"')


def list_all(collection, filt):
    out, page = [], 1
    while True:
        q = urllib.parse.urlencode({"filter": filt, "perPage": 500, "page": page})
        got = call("GET", f"/api/collections/{collection}/records?{q}")
        out += got.get("items", [])
        if page >= got.get("totalPages", 1):
            return out
        page += 1


# ---- el mundo inventado -----------------------------------------------------

# Por tema: de qué tratan sus hilos. El tema es el primer tag más común.
ASPECTS = {
    "pagos": ["tarjetas guardadas", "reintentos de cobro", "conciliación diaria", "reembolsos parciales", "pagos recurrentes", "3-D Secure", "comisiones por pasarela", "cobro en efectivo"],
    "facturación": ["timbrado CFDI 4.0", "notas de crédito", "complemento de pago", "cancelación de facturas", "series y folios", "facturas globales"],
    "onboarding": ["registro sin contraseña", "tour de la primera sesión", "verificación de teléfono", "invitar al equipo", "plantillas iniciales", "correo de bienvenida"],
    "móvil": ["modo sin conexión", "notificaciones push", "permisos de cámara", "deep links", "tamaño del paquete", "arranque en frío"],
    "web": ["menú de navegación", "accesibilidad AA", "modo oscuro", "tablas con filtros", "carga diferida", "formularios largos"],
    "infraestructura": ["Postgres 17", "réplicas de lectura", "colas de trabajo", "límites de memoria", "despliegue azul-verde", "costos de nube"],
    "seguridad": ["rotación de secretos", "SSO con SAML", "auditoría de accesos", "2FA obligatorio", "escaneo de dependencias", "política de contraseñas"],
    "datos": ["ingesta diaria", "catálogo de tablas", "panel de uso", "exportar a CSV", "retención de datos", "modelo de cohortes"],
    "observabilidad": ["trazas distribuidas", "alertas de latencia", "tableros de errores", "logs estructurados", "SLO de disponibilidad", "presupuesto de errores"],
    "soporte": ["macros de respuesta", "encuesta de satisfacción", "guardias de fin de semana", "base de conocimiento", "escalamiento a nivel 2", "tiempos de primera respuesta"],
    "calidad": ["pruebas de regresión visual", "pruebas de contrato", "muestreo de tickets", "cobertura de pruebas", "pruebas de carga", "checklist de release"],
    "integraciones": ["webhook de pedidos", "API pública v2", "conector con el ERP", "sincronizar inventario", "límites de tasa", "SDK de JavaScript"],
    "ventas": ["pipeline trimestral", "demos guiadas", "propuesta comercial", "precios por volumen", "seguimiento de prospectos", "renovaciones"],
    "marketing": ["campaña de lanzamiento", "página de precios", "casos de éxito", "boletín mensual", "SEO del blog", "webinar con clientes"],
    "legal": ["términos de servicio", "aviso de privacidad", "contrato marco", "acuerdo de procesamiento de datos", "registro de marca"],
    "diseño": ["sistema de componentes", "iconografía", "estados vacíos", "guía de tono", "prototipo del tablero", "tipografía"],
}
VERBS = ["Diseñar", "Implementar", "Probar", "Documentar", "Medir", "Migrar", "Revisar", "Corregir", "Automatizar", "Definir", "Simplificar", "Publicar"]
CLIENTS = ["cliente/acme", "cliente/globex", "cliente/initech", "cliente/umbrella"]
EXTRA = ["urgente", "deuda-técnica", "q4", "experimento", "regulatorio", "rendimiento"]

TEAMS = {
    "Demo Producto": {
        "demo-app-clientes": ("Demo App Clientes", "móvil", ["pagos", "onboarding", "móvil", "calidad", "seguridad", "diseño", "integraciones"]),
        "demo-portal-web": ("Demo Portal Web", "web", ["facturación", "datos", "onboarding", "seguridad", "web", "diseño", "pagos"]),
        "demo-checkout": ("Demo Checkout", "web", ["pagos", "facturación", "calidad", "integraciones", "observabilidad"]),
        "demo-sistema-de-diseno": ("Demo Sistema de Diseño", "diseño", ["diseño", "web", "móvil", "calidad"]),
    },
    "Demo Operaciones": {
        "demo-plataforma": ("Demo Plataforma", "plataforma", ["infraestructura", "seguridad", "datos", "observabilidad", "calidad", "integraciones"]),
        "demo-soporte": ("Demo Soporte", "soporte", ["soporte", "pagos", "facturación", "calidad", "datos"]),
        "demo-observabilidad": ("Demo Observabilidad", "plataforma", ["observabilidad", "infraestructura", "datos", "calidad"]),
        "demo-seguridad": ("Demo Seguridad", "plataforma", ["seguridad", "legal", "infraestructura", "observabilidad"]),
    },
    "Demo Comercial": {
        "demo-ventas": ("Demo Ventas", "comercial", ["ventas", "legal", "facturación", "datos", "integraciones"]),
        "demo-marketing": ("Demo Marketing", "comercial", ["marketing", "web", "diseño", "datos", "ventas"]),
        "demo-alianzas": ("Demo Alianzas", "comercial", ["integraciones", "legal", "ventas", "marketing"]),
    },
}

WHY = [
    "Hoy se resuelve a mano y se equivoca una de cada diez veces; queremos que no dependa de quién esté de guardia.",
    "Los clientes lo piden en casi todas las llamadas de renovación y es lo que más tickets abre.",
    "Bloquea la siguiente entrega: sin esto no podemos activar el resto del flujo.",
    "Es un requisito del regulador para el cierre del trimestre.",
    "Medimos que es la principal causa de abandono en esta parte del producto.",
    "Lo arrastramos desde hace dos ciclos y cada parche lo hace más frágil.",
]
DONE = [
    "Funciona en producción para al menos un cliente real",
    "Las pruebas automáticas cubren el caso feliz y los dos errores más comunes",
    "Soporte sabe explicarlo sin escalar",
    "Hay una métrica en el tablero que dice si funciona",
    "La documentación pública está actualizada",
    "Revisado por alguien de otro equipo",
]
MORE = [
    "Probamos primero con un cliente piloto y medimos durante dos semanas antes de abrirlo a todos.",
    "La decisión de diseño fue mantener el formato actual y agregar un campo opcional, para no romper integraciones.",
    "Quedan dudas sobre el volumen en temporada alta; si pasa de mil eventos por minuto habría que partir la cola.",
    "Soporte pidió que el mensaje de error diga qué hacer, no sólo qué falló.",
    "Lo hablamos con finanzas: el corte contable no cambia, sólo cuándo se concilia.",
    "Hay que revisar los permisos: hoy cualquier persona del proyecto puede verlo y quizá no debería.",
    "El riesgo principal es la migración de datos viejos; vale más ensayarla dos veces en staging.",
    "Documentar el procedimiento de vuelta atrás antes de activar nada en producción.",
]
NOTES = [
    "El proveedor responde en 48 h; conviene abrir el caso antes de empezar.",
    "Hay una versión vieja en una rama que se puede reutilizar a medias.",
    "Ojo con la zona horaria: los cortes son a medianoche de la Ciudad de México.",
    "Se habló en la retro del ciclo pasado: el riesgo es el volumen de datos, no la lógica.",
    "Pendiente confirmar con legal antes de publicarlo.",
    "",
]


def world():
    """El plan entero, determinista: lo mismo cada vez que se corre."""
    rng = random.Random(20261020)
    plan = []
    for team, projects in TEAMS.items():
        for slug, (ws_name, area, topics) in projects.items():
            bubbles = []
            used = set()  # un nombre de hilo no se repite en el proyecto
            for topic in topics:
                for k in range(rng.choice([1, 1, 2])):
                    aspects = ASPECTS[topic][:]
                    rng.shuffle(aspects)
                    b_name = f"{aspects[0][0].upper()}{aspects[0][1:]}" if k == 0 else f"{topic.capitalize()}: segunda etapa"
                    threads = []
                    for i in range(rng.randint(3, 8)):
                        aspect = aspects[i % len(aspects)]
                        name = f"{VERBS[(i + len(b_name)) % len(VERBS)]} {aspect}"
                        if name in used:
                            continue
                        used.add(name)
                        roll = rng.random()
                        if roll < 0.1:
                            tags = []  # sin frontmatter: va a «Sin tags»
                        elif roll < 0.25:
                            other = rng.choice([t for t in ASPECTS if t != topic])
                            tags = [other, topic]  # su primer tag no es el de la burbuja
                        else:
                            tags = [topic, area]
                        if tags and rng.random() < 0.3:
                            tags.append(rng.choice(CLIENTS))
                        if tags and rng.random() < 0.15:
                            tags.append(rng.choice(EXTRA))
                        # Lo escrito de más (párrafos, páginas, imágenes) sale de su
                        # propio generador: no altera el resto del plan, así que lo
                        # ya sembrado con versiones anteriores sigue coincidiendo.
                        xr = random.Random(f"{slug}/{name}")
                        threads.append({
                            "extra": xr.sample(MORE, xr.choice([0, 0, 1, 2, 3, 5, 8])),
                            "pages": [xr.sample(MORE, xr.randint(1, 6)) for _ in range(xr.choice([0, 0, 0, 0, 1, 1, 2, 3]))],
                            "images": sorted(xr.sample(range(3), xr.choice([0, 0, 0, 1, 1, 2]))),
                            "name": name, "tags": list(dict.fromkeys(tags)),
                            "done": rng.random() < 0.15,
                            "why": rng.choice(WHY), "dod": rng.sample(DONE, 3),
                            "ticked": rng.randint(0, 3), "note": rng.choice(NOTES),
                        })
                    bubbles.append({
                        "name": b_name, "owned": rng.random() < 0.75,
                        "closed": rng.random() < 0.08, "threads": threads,
                    })
            plan.append((team, slug, ws_name, bubbles))
    return plan


def document(t, slug):
    front = f"---\ntags: [{', '.join(t['tags'])}]\n---\n" if t["tags"] else ""
    boxes = "\n".join(f"- [{'x' if i < t['ticked'] else ' '}] {d}" for i, d in enumerate(t["dod"]))
    note = f"\n## Notas\n\n{t['note']}\n" if t["note"] else ""
    more = "".join(f"\n{p}\n" for p in t["extra"])
    pics = "".join(f"\n![Diagrama {i + 1}](assets/{slug}-diagrama-{i + 1}.png)\n" for i in t["images"])
    return f"{front}# {t['name']}\n\n## Para qué\n\n{t['why']}\n{more}\n## Hecho cuando\n\n{boxes}\n{note}{pics}"


def page(t, i):
    body = "\n\n".join(t["pages"][i])
    return f"# Notas {i + 1}\n\n{body}\n"


def png(rgb):
    """Una imagen de 48×32 de un solo color, sin dependencias."""
    w, h = 48, 32
    raw = b"".join(b"\x00" + bytes(rgb) * w for _ in range(h))
    chunk = lambda tag, data: struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")


def upload(ws_id, name, data):
    boundary = uuid.uuid4().hex
    body = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{name}\"\r\nContent-Type: image/png\r\n\r\n").encode() + data + f"\r\n--{boundary}--\r\n".encode()
    req = urllib.request.Request(f"{API}/api/workspaces/{ws_id}/asset", data=body, method="POST")
    req.add_header("Content-Type", f"multipart/form-data; boundary={boundary}")
    req.add_header("Authorization", TOKEN)
    with urllib.request.urlopen(req) as res:
        return json.loads(res.read())


EMPTY = hashlib.sha256(b"").hexdigest()[:32]


def main():
    global TOKEN
    auth = call("POST", "/api/collections/users/auth-with-password", {"identity": EMAIL, "password": PASSWORD})
    TOKEN = auth["token"]
    me = auth["record"]
    if me.get("role") != "lead":
        sys.exit("seed: la persona de .env tiene que ser lead global (crea equipos)")
    made = {"equipos": 0, "proyectos": 0, "burbujas": 0, "hilos": 0, "documentos": 0, "páginas": 0, "imágenes": 0, "completados": 0, "relaciones": 0}
    teams = {t["name"]: t["id"] for t in call("GET", "/api/teams")["teams"]}
    rng = random.Random(7)
    by_team = {}

    for team_name, slug, ws_name, bubbles in world():
        if team_name not in teams:
            teams[team_name] = call("POST", "/api/teams", {"name": team_name})["id"]
            made["equipos"] += 1
        found = list_all("workspaces", f'slug = "{slug}"')
        ws = found[0] if found else call("POST", "/api/collections/workspaces/records", {"name": ws_name, "slug": slug, "team": teams[team_name]})
        made["proyectos"] += not found
        # Tres imágenes por proyecto, para que los hilos las citen.
        have_assets = {e["path"] for e in call("GET", f"/api/workspaces/{ws['id']}/tree").get("entries", [])}
        for i, rgb in enumerate([(232, 93, 117), (96, 165, 250), (52, 211, 153)]):
            name = f"{slug}-diagrama-{i + 1}.png"
            if f"assets/{name}" not in have_assets:
                upload(ws["id"], name, png(rgb))
                made["imágenes"] += 1
        done_state = next((s["id"] for s in list_all("states", f'workspace = "{ws["id"]}" && group = "completed"')), "")
        have_b = {b["name"]: b for b in list_all("bubbles", f'workspace = "{ws["id"]}"')}
        have_t = {t["name"]: t for t in list_all("threads", f'workspace = "{ws["id"]}"')}
        for b in bubbles:
            rec = have_b.get(b["name"])
            if not rec:
                rec = call("POST", "/api/collections/bubbles/records", {
                    "workspace": ws["id"], "name": b["name"],
                    "outcome": f"{b['name']}: en producción y medido.",
                    "owners": [me["id"]] if b["owned"] else [],
                })
                made["burbujas"] += 1
            for t in b["threads"]:
                th = have_t.get(t["name"])
                if not th:
                    th = call("POST", "/api/collections/threads/records", {"workspace": ws["id"], "bubble": rec["id"], "name": t["name"]})
                    made["hilos"] += 1
                by_team.setdefault(team_name, []).append((th["id"], slug, b["name"]))
                content = document(t, slug)
                doc = call("GET", f"/api/threads/{th['id']}/document")
                if doc.get("content") != content:
                    call("PATCH", f"/api/threads/{th['id']}/document", {"base": doc["hash"], "content": content})
                    made["documentos"] += 1
                folder = (th.get("doc_path") or call("GET", f"/api/collections/threads/records/{th['id']}")["doc_path"]).removesuffix(".md")
                for i in range(len(t["pages"])):
                    p = f"{folder}/notas-{i + 1}.md"
                    q = urllib.parse.quote(p)
                    cur = call("GET", f"/api/workspaces/{ws['id']}/document?path={q}")
                    if cur.get("content") != page(t, i):
                        call("PATCH", f"/api/workspaces/{ws['id']}/document", {"path": p, "base": cur.get("hash") or EMPTY, "content": page(t, i)})
                        made["páginas"] += 1
                if t["done"] and done_state and th.get("state") != done_state:
                    call("PATCH", f"/api/collections/threads/records/{th['id']}", {"state": done_state})
                    made["completados"] += 1
            if b["closed"] and not rec.get("closed_at"):
                call("PATCH", f"/api/collections/bubbles/records/{rec['id']}", {
                    "closed_at": datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S.000Z"),
                    "closure": "Se cerró: el resultado ya no justificaba el trabajo este trimestre."})

    # Relaciones como las de verdad: casi todas cercanas. Cadenas de
    # dependencia dentro de una burbuja (un paso espera al anterior), algunas
    # entre burbujas del mismo proyecto y pocas entre proyectos del equipo.
    # Nunca entre equipos: esas no son aristas.
    planned = set()
    for team_name, rows in by_team.items():
        rows = sorted(set(rows))
        by_bubble, by_project = {}, {}
        for tid, slug, b_name in rows:
            by_bubble.setdefault((slug, b_name), []).append(tid)
            by_project.setdefault(slug, []).append(tid)
        for key in sorted(by_bubble):
            ids = by_bubble[key]
            if len(ids) >= 2 and rng.random() < 0.7:
                for i in range(min(len(ids) - 1, rng.randint(1, 3))):
                    planned.add((ids[i], ids[i + 1], "blocking"))
        for slug in sorted(by_project):
            ids = by_project[slug]
            for _ in range(len(ids) // 8):
                a, b = rng.sample(ids, 2)
                planned.add((a, b, rng.choice(["blocking", "relates_to", "relates_to"])))
        team_ids = [r[0] for r in rows]
        for _ in range(len(team_ids) // 30):
            a, b = rng.sample(team_ids, 2)
            planned.add((a, b, "relates_to"))
    # Lo que se dice una sola vez, en un sentido.
    want = {}
    for a, b, kind in sorted(planned):
        if (b, a) not in want and (a, b) not in want:
            want[(a, b)] = kind

    # Limpieza: relaciones entre hilos de este seed que ya no están en el plan
    # (corridas anteriores con otra regla). Lo que no es del seed no se toca.
    ours = {r[0] for rows in by_team.values() for r in rows}
    seen = set()
    for rel in list_all("thread_relations", 'thread.workspace.slug ~ "demo-"'):
        a, b = rel["thread"], rel["related"]
        if a in ours and b in ours:
            if (a, b) in want or (b, a) in want:
                seen.add((a, b) if (a, b) in want else (b, a))
            else:
                call("DELETE", f"/api/collections/thread_relations/records/{rel['id']}")
                made["relaciones quitadas"] = made.get("relaciones quitadas", 0) + 1
    for (a, b), kind in want.items():
        if (a, b) in seen:
            continue
        try:
            call("POST", "/api/collections/thread_relations/records", {"thread": a, "related": b, "type": kind})
            made["relaciones"] += 1
        except RuntimeError:
            pass  # una combinación que el servidor rechaza no detiene la siembra

    print("seed docs:", ", ".join(f"{v} {k}" for k, v in made.items()), "nuevos o reescritos")
    print(f"ábrelo en Todos los equipos → Blobs ({API})")


if __name__ == "__main__":
    main()
