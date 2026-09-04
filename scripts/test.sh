#!/usr/bin/env bash
# Every test: the Go ones, then the ones that need a running server.
#
#   just test
#
# The second half is here because an API rule is not a function you can call.
# Everything runs against a throwaway data directory on its own port.
#
# What it is really guarding: nothing supplies the workspace boundary any more, so
# every collection is insecure until its rule says which workspace it belongs to.
# The >>> lines are the ones that would be security bugs rather than wrong answers.
#
# These want to be Go tests eventually — PocketBase ships a `tests` package for
# exactly this. Curl is what they are until somebody moves them.
set -uo pipefail
cd "$(dirname "$0")/.."

echo "==> go test"
go test -race ./... || exit 1

echo "==> live"
D=${TMPDIR:-/tmp}/bubble-test-data
B=./dist/bubble
API=http://127.0.0.1:8099
R=${TMPDIR:-/tmp}/bubble-test-repos

# Kill only OUR leftover, matched by the test port — never a bare
# `pkill -f "bubble serve"`.
#
# That pattern matches every server on the machine, including the one somebody is
# looking at in a browser. It ran at the start AND in the trap, so every
# `just check` took down a running `just dev` twice. The tests are supposed to be
# invisible to whoever is working.
pkill -f "bubble serve.*8099" 2>/dev/null; sleep 0.3
rm -rf "$D" "$R"
[[ -x "$B" ]] || scripts/build.sh

pass=0; fail=0
ok(){ echo "  PASS  $1"; pass=$((pass+1)); }
no(){ echo "  FAIL  $1  -> $2"; fail=$((fail+1)); }
chk(){ if [ "$2" = "$3" ]; then ok "$1"; else no "$1" "esperaba $3, dio $2"; fi; }

"$B" superuser upsert root@bubble.test rootrootroot --dir "$D" >/dev/null 2>&1
BUBBLE_REPOS="$R" "$B" serve --http 127.0.0.1:8099 --dir "$D" >/tmp/bubble-test.log 2>&1 &
SRV=$!
# By PID: the only process this script is entitled to end is the one it started.
trap 'kill "$SRV" 2>/dev/null' EXIT
for i in $(seq 1 60); do curl -sf "$API/api/health" >/dev/null 2>&1 && break; sleep 0.25; done
if ! curl -sf "$API/api/health" >/dev/null 2>&1; then
  echo "  el server no arrancó — ver /tmp/bubble-test.log" >&2; exit 1
fi

j(){ python3 -c 'import sys,json
try:
    d=json.load(sys.stdin); exec("v=d"+sys.argv[1]); print(v)
except Exception: print("")' "$1"; }
code(){ curl -s -o /dev/null -w '%{http_code}' "$@"; }
JS='Content-Type: application/json'
post(){ curl -s -X POST "$API/api/collections/$1/records" -H "Authorization: $2" -H "$JS" -d "$3"; }
pcode(){ code -X POST "$API/api/collections/$1/records" -H "Authorization: $2" -H "$JS" -d "$3"; }
list(){ curl -s "$API/api/collections/$1/records" -H "Authorization: $2"; }

# ---------------------------------------------------------------- phase 0 ----
SU=$(curl -s -X POST "$API/api/collections/_superusers/auth-with-password" -H "$JS" \
  -d '{"identity":"root@bubble.test","password":"rootrootroot"}' | j "['token']")
[ -n "$SU" ] && ok "superuser autentica" || { no "superuser autentica" "sin token"; exit 1; }

# Guard against talking to somebody else's server on this port. A leftover process
# from an earlier run answers every request with an OLDER database, which does not
# fail — it passes, against data this run never created. Chasing that once was
# enough: a wrong-server is now loud.
FRESH=$(curl -s "$API/api/collections/workspaces/records" -H "Authorization: $SU" | j "['totalItems']")
if [ "$FRESH" != "0" ]; then
  echo "  el server en $API no es el nuestro (ya tiene datos: workspaces=$FRESH)" >&2
  echo "  mata lo que esté escuchando ahí y vuelve a correr" >&2
  exit 1
fi
ok "el server es nuestro y está vacío"

mkuser(){ post users "$SU" "{\"email\":\"$1\",\"password\":\"passwordpass\",\"passwordConfirm\":\"passwordpass\",\"display_name\":\"$2\",\"role\":\"${3:-member}\",\"verified\":true}"; }
login(){ curl -s -X POST "$API/api/collections/users/auth-with-password" -H "$JS" \
  -d "{\"identity\":\"$1\",\"password\":\"passwordpass\"}" | j "['token']"; }

AID=$(mkuser alice@bubble.test Alice | j "['id']")
BID=$(mkuser bob@bubble.test Bob     | j "['id']")
CID=$(mkuser carol@bubble.test Carol lead | j "['id']")   # lead GLOBAL, de ningún workspace
if [ -n "$AID" ] && [ -n "$BID" ] && [ -n "$CID" ]; then ok "users acepta display_name y role"; else no "crear users" "$AID/$BID/$CID"; fi
A=$(login alice@bubble.test); B=$(login bob@bubble.test); C=$(login carol@bubble.test)
if [ -n "$A" ] && [ -n "$B" ] && [ -n "$C" ]; then ok "alice, bob y carol autentican"; else no "login" "vacío"; fi

ALPHA=$(post workspaces "$A" '{"name":"Alpha","slug":"alpha"}' | j "['id']")
BETA=$(post  workspaces "$B" '{"name":"Beta","slug":"beta"}'   | j "['id']")
if [ -n "$ALPHA" ] && [ -n "$BETA" ]; then ok "alice y bob crean workspace"; else no "crear workspace" "$ALPHA/$BETA"; fi

MS=$(curl -s "$API/api/collections/memberships/records?filter=(workspace='$ALPHA')" -H "Authorization: $SU")
if [ "$(echo "$MS"|j "['totalItems']")" = "1" ] && [ "$(echo "$MS"|j "['items'][0]['role']")" = "lead" ] \
   && [ "$(echo "$MS"|j "['items'][0]['user']")" = "$AID" ]; then
  ok "membresía fundadora: 1 fila, role=lead, user=el creador"
else no "membresía fundadora" "$(echo "$MS"|head -c 200)"; fi

WA=$(list workspaces "$A"); WB=$(list workspaces "$B")
if [ "$(echo "$WA"|j "['totalItems']")" = "1" ] && [ "$(echo "$WA"|j "['items'][0]['slug']")" = "alpha" ] \
   && [ "$(echo "$WB"|j "['items'][0]['slug']")" = "beta" ]; then
  ok "cada quien lista SOLO su workspace"
else no "aislamiento de lista" "A=$(echo "$WA"|j "['totalItems']") B=$(echo "$WB"|j "['totalItems']")"; fi

chk "alice no ve beta por id" "$(code "$API/api/collections/workspaces/records/$BETA" -H "Authorization: $A")" 404
chk "anónimo lista 0 workspaces" "$(curl -s "$API/api/collections/workspaces/records" | j "['totalItems']")" 0

INV=$(pcode memberships "$A" "{\"workspace\":\"$ALPHA\",\"user\":\"$BID\",\"role\":\"member\"}")
chk ">>> alice (lead del workspace) SÍ puede invitar" "$INV" 200
chk "bob (member) ahora ve alpha y beta" "$(list workspaces "$B" | j "['totalItems']")" 2
chk ">>> bob (member raso) NO puede invitar" \
  "$(pcode memberships "$B" "{\"workspace\":\"$ALPHA\",\"user\":\"$CID\",\"role\":\"member\"}")" 400

echo
chk ">>> bob (member) NO puede editar alpha  [las dos ?= atan a la MISMA fila]" \
  "$(code -X PATCH "$API/api/collections/workspaces/records/$ALPHA" -H "Authorization: $B" -H "$JS" -d '{"name":"Secuestrado"}')" 404
chk ">>> alice (lead) SÍ puede editar alpha" \
  "$(code -X PATCH "$API/api/collections/workspaces/records/$ALPHA" -H "Authorization: $A" -H "$JS" -d '{"name":"Alpha renombrado"}')" 200
echo

chk "alice ve el roster de alpha (2), no el de beta" "$(list memberships "$A" | j "['totalItems']")" 2
chk "membresía duplicada rechazada (índice único)" "$(pcode memberships "$A" "{\"workspace\":\"$ALPHA\",\"user\":\"$BID\",\"role\":\"lead\"}")" 400
chk "slug duplicado rechazado" "$(pcode workspaces "$A" '{"name":"Otro","slug":"alpha"}')" 400
chk "slug con espacios/mayúsculas rechazado" "$(pcode workspaces "$A" '{"name":"Otro","slug":"Con Espacios"}')" 400

# --------------------------------------------------------------- phase 1a ----
echo
ST_A=$(post states "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"En curso\",\"group\":\"started\",\"position\":1}" | j "['id']")
LB_A=$(post labels "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"bug\",\"color\":\"#f00\"}" | j "['id']")
BU_A=$(post bubbles "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Primera burbuja\",\"outcome\":\"algo cambió\"}" | j "['id']")
BU_B=$(post bubbles "$B" "{\"workspace\":\"$BETA\",\"name\":\"Burbuja de beta\"}" | j "['id']")
if [ -n "$ST_A" ] && [ -n "$LB_A" ] && [ -n "$BU_A" ] && [ -n "$BU_B" ]; then
  ok "state, label y bubbles creados"; else no "crear estructura" "$ST_A/$LB_A/$BU_A/$BU_B"; fi

T1=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"bubble\":\"$BU_A\",\"name\":\"Primer thread\",\"state\":\"$ST_A\",\"labels\":[\"$LB_A\"],\"impact\":\"high\",\"urgency\":\"high\"}")
T2=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"bubble\":\"$BU_A\",\"name\":\"Segundo\"}")
T3=$(post threads "$B" "{\"workspace\":\"$BETA\",\"bubble\":\"$BU_B\",\"name\":\"Primero de beta\"}")
T1ID=$(echo "$T1"|j "['id']")
if [ "$(echo "$T1"|j "['seq']")" = "1" ] && [ "$(echo "$T2"|j "['seq']")" = "2" ]; then
  ok "seq se asigna en el server: 1, 2"; else no "seq" "$(echo "$T1"|j "['seq']")/$(echo "$T2"|j "['seq']")"; fi
chk "seq reinicia por workspace (beta arranca en 1)" "$(echo "$T3"|j "['seq']")" 1
T4=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Con seq falso\",\"seq\":99}")
chk "el cliente no puede imponer su seq" "$(echo "$T4" | j "['seq']")" 3

echo
chk ">>> thread de beta con una bubble de ALPHA se rechaza" \
  "$(pcode threads "$B" "{\"workspace\":\"$BETA\",\"bubble\":\"$BU_A\",\"name\":\"Colado\"}")" 400
chk ">>> bob (member) NO puede crear un state en alpha (config = lead)" \
  "$(pcode states "$B" "{\"workspace\":\"$ALPHA\",\"name\":\"Colado\",\"group\":\"backlog\"}")" 400
chk ">>> bob (member) SÍ puede crear un label en alpha" \
  "$(pcode labels "$B" "{\"workspace\":\"$ALPHA\",\"name\":\"chore\"}")" 200
chk ">>> bob (member) NO puede borrar una bubble de alpha (lead)" \
  "$(code -X DELETE "$API/api/collections/bubbles/records/$BU_A" -H "Authorization: $B")" 404
echo

# alice no es miembro de beta: nada de beta le existe
chk "alice no ve bubbles de beta" "$(list bubbles "$A" | j "['totalItems']")" 1
chk "alice no ve threads de beta" "$(list threads "$A" | j "['totalItems']")" 3
chk "alice no ve la bubble de beta por id" \
  "$(code "$API/api/collections/bubbles/records/$BU_B" -H "Authorization: $B")" 200
chk "...y alice sí recibe 404 por esa misma" \
  "$(code "$API/api/collections/bubbles/records/$BU_B" -H "Authorization: $A")" 404
chk "anónimo no ve threads" "$(curl -s "$API/api/collections/threads/records" | j "['totalItems']")" 0

# autoría: el server la impone, el cliente no la elige
C1=$(post comments "$B" "{\"thread\":\"$T1ID\",\"author\":\"$AID\",\"body\":\"firmado como alice\"}")
chk ">>> el comentario de bob queda firmado por BOB aunque pidió alice" "$(echo "$C1"|j "['author']")" "$BID"
C1ID=$(echo "$C1"|j "['id']")
chk ">>> alice no puede editar el comentario de bob" \
  "$(code -X PATCH "$API/api/collections/comments/records/$C1ID" -H "Authorization: $A" -H "$JS" -d '{"body":"editado"}')" 404
chk "bob sí puede editar el suyo" \
  "$(code -X PATCH "$API/api/collections/comments/records/$C1ID" -H "Authorization: $B" -H "$JS" -d '{"body":"editado"}')" 200

L1=$(post thread_links "$B" "{\"thread\":\"$T1ID\",\"url\":\"https://example.com/pr/1\",\"title\":\"PR\",\"added_by\":\"$AID\"}")
chk "el link queda a nombre de quien lo puso, no de quien dijo" "$(echo "$L1"|j "['added_by']")" "$BID"

R1=$(pcode thread_relations "$A" "{\"thread\":\"$T1ID\",\"type\":\"blocking\",\"related\":\"$(echo "$T2"|j "['id']")\"}")
chk "relación creada" "$R1" 200
chk "relación duplicada rechazada (índice único)" \
  "$(pcode thread_relations "$A" "{\"thread\":\"$T1ID\",\"type\":\"blocking\",\"related\":\"$(echo "$T2"|j "['id']")\"}")" 400

# borrar una bubble NO debe llevarse la escritura de sus threads
curl -s -X DELETE "$API/api/collections/bubbles/records/$BU_A" -H "Authorization: $A" >/dev/null
chk ">>> borrar la bubble deja vivos sus threads" "$(list threads "$A" | j "['totalItems']")" 3
chk "...y el thread queda sin bubble" \
  "$(curl -s "$API/api/collections/threads/records/$T1ID" -H "Authorization: $A" | j "['bubble']")" ""


# ------------------------------------------------------- lead global ----
echo
chk ">>> carol (lead global) ve LOS DOS workspaces sin ser miembro" "$(list workspaces "$C" | j "['totalItems']")" 2
chk ">>> ...y un miembro raso sigue SIN cruzar (bob ve 2 porque es de ambos)" "$(list workspaces "$B" | j "['totalItems']")" 2
DAVE=$(mkuser dave@bubble.test Dave | j "['id']")
DV=$(login dave@bubble.test)
chk ">>> dave, sin membresías, ve 0 workspaces  [el prefijo no abrió todo]" "$(list workspaces "$DV" | j "['totalItems']")" 0
chk ">>> dave ve 0 threads" "$(list threads "$DV" | j "['totalItems']")" 0
chk "carol (lead global) ve los threads de ambos" "$(list threads "$C" | j "['totalItems']")" 4
chk "carol puede renombrar un workspace del que no es miembro" \
  "$(code -X PATCH "$API/api/collections/workspaces/records/$BETA" -H "Authorization: $C" -H "$JS" -d '{"name":"Beta por carol"}')" 200
chk "cualquiera logueado puede listar personas (para poder invitar)" \
  "$([ "$(list users "$DV" | j "['totalItems']")" -ge 4 ] && echo si || echo no)" si

echo
# 404 y no 400: la regla FILTRA, no rechaza — para dave ese registro deja de
# existir en cuanto el cuerpo trae `role`. Invisible en vez de prohibido.
chk ">>> dave NO puede autopromoverse a lead global" \
  "$(code -X PATCH "$API/api/collections/users/records/$DAVE" -H "Authorization: $DV" -H "$JS" -d '{"role":"lead"}')" 404
chk "dave sí puede editar su propio nombre" \
  "$(code -X PATCH "$API/api/collections/users/records/$DAVE" -H "Authorization: $DV" -H "$JS" -d '{"display_name":"Dave B"}')" 200
chk "carol (lead global) sí puede promover a dave" \
  "$(code -X PATCH "$API/api/collections/users/records/$DAVE" -H "Authorization: $C" -H "$JS" -d '{"role":"lead"}')" 200

# ------------------------------------------------------ último lead ----
echo
MA=$(curl -s "$API/api/collections/memberships/records?filter=(workspace=%27$ALPHA%27)" -H "Authorization: $SU")
LEADROW=$(echo "$MA" | python3 -c 'import sys,json
d=json.load(sys.stdin)
print(next((i["id"] for i in d["items"] if i["role"]=="lead"), ""))')
chk ">>> el ÚLTIMO lead no puede degradarse" \
  "$(code -X PATCH "$API/api/collections/memberships/records/$LEADROW" -H "Authorization: $A" -H "$JS" -d '{"role":"member"}')" 400
chk ">>> el ÚLTIMO lead no puede borrar su membresía" \
  "$(code -X DELETE "$API/api/collections/memberships/records/$LEADROW" -H "Authorization: $A")" 400
BOBROW=$(echo "$MA" | python3 -c 'import sys,json
d=json.load(sys.stdin)
print(next((i["id"] for i in d["items"] if i["role"]=="member"), ""))')
chk "alice promueve a bob a lead" \
  "$(code -X PATCH "$API/api/collections/memberships/records/$BOBROW" -H "Authorization: $A" -H "$JS" -d '{"role":"lead"}')" 200
chk "con dos leads, alice YA puede degradarse" \
  "$(code -X PATCH "$API/api/collections/memberships/records/$LEADROW" -H "Authorization: $A" -H "$JS" -d '{"role":"member"}')" 200


# ------------------------------------------- documentos: el árbol de markdown ----
#
# Las peticiones que MUTAN se arman en una variable y se llaman directo, nunca
# dentro de $( ) como argumento de chk. Tres veces en esta sesión una llamada así
# no ocurrió y la aserción pasó de todas formas — con el cuerpo llegando vacío al
# curl, que el server contesta 200 sin escribir nada. Un test que silenciosamente
# no corre es peor que uno que falla.
echo
GETDOC="$API/api/threads/$T1ID/document"
getdoc(){ curl -s "$GETDOC" -H "Authorization: $1"; }
patchdoc(){ # $1 token, $2 body -> imprime el código; el cuerpo queda en /tmp/bubble-put.json
  curl -s -o /tmp/bubble-put.json -w '%{http_code}' -X PATCH "$GETDOC" \
    -H "Authorization: $1" -H "$JS" -d "$2"; }

WSA=$(curl -s "$API/api/collections/workspaces/records/$ALPHA" -H "Authorization: $SU")
chk "el workspace tiene repo_path" "$(echo "$WSA" | j "['repo_path']")" "alpha"
TH1=$(curl -s "$API/api/collections/threads/records/$T1ID" -H "Authorization: $SU")
chk "el thread tiene doc_path derivado de seq y nombre" \
  "$(echo "$TH1" | j "['doc_path']")" "threads/1-primer-thread.md"

D0=$(getdoc "$A")
chk "un documento que no existe se lee vacío" "$(echo "$D0" | j "['content']")" ""
H0=$(echo "$D0" | j "['hash']")

BODY="{\"content\":\"# Uno\\n\\n- [ ] algo\\n\",\"base\":\"$H0\",\"message\":\"born: uno\"}"
CODE=$(patchdoc "$A" "$BODY")
chk "primera escritura con el hash de vacío" "$CODE" 200
D1=$(getdoc "$A")
chk "y se lee de vuelta" "$(echo "$D1" | j "['content']" | head -c 5)" "# Uno"
H1=$(echo "$D1" | j "['hash']")

BODY="{\"content\":\"pisado\",\"base\":\"$H0\"}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> escribir con un base viejo es CONFLICTO, no sobreescritura" "$CODE" 400
chk "...y el contenido sigue intacto" "$(getdoc "$A" | j "['content']" | head -c 5)" "# Uno"

BODY="{\"content\":\"# Uno\\n\\n- [x] algo\\n\",\"base\":\"$H1\",\"message\":\"tick\"}"
CODE=$(patchdoc "$A" "$BODY")
chk "con el base correcto sí escribe" "$CODE" 200
H2=$(python3 -c 'import json;print(json.load(open("/tmp/bubble-put.json"))["hash"])')

# --- el PATCH único: tres formas, un solo endpoint ---
echo
BODY="{\"base\":\"$H2\",\"edits\":[{\"old\":\"# Uno\",\"new\":\"# Uno editado\"}]}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> edits: parche por contexto" "$CODE" 200
chk "...aplicado" "$(getdoc "$A" | j "['content']" | head -c 13)" "# Uno editado"
H3=$(python3 -c 'import json;print(json.load(open("/tmp/bubble-put.json"))["hash"])')

BODY="{\"base\":\"$H3\",\"edits\":[{\"old\":\"no existe en el documento\",\"new\":\"x\"}]}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> un edit que no encuentra su texto se rechaza" "$CODE" 400
chk "...con un mensaje accionable" \
  "$(python3 -c 'import json;print("si" if "not in this section" in json.load(open("/tmp/bubble-put.json"))["message"] else "no")')" si

BODY="{\"base\":\"$H3\",\"edits\":[{\"old\":\"o\",\"new\":\"0\"}]}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> un edit ambiguo se rechaza en vez de adivinar" "$CODE" 400
chk "...y dice que cites más" \
  "$(python3 -c 'import json;print("si" if "quote more" in json.load(open("/tmp/bubble-put.json"))["message"] else "no")')" si

# La casilla viene marcada de la escritura anterior, así que desmarcarla es lo
# que de verdad cambia el archivo. Marcar lo ya marcado no cambia nada y por eso
# no produce commit — eso se comprueba aparte, abajo.
BODY="{\"base\":\"$H3\",\"todo\":{\"index\":0,\"done\":false}}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> todo: desmarcar una casilla por índice" "$CODE" 200
chk "...y devuelve cuántas van hechas" \
  "$(python3 -c 'import json;print(json.load(open("/tmp/bubble-put.json"))["done"])')" 0
H4=$(python3 -c 'import json;print(json.load(open("/tmp/bubble-put.json"))["hash"])')

BODY="{\"base\":\"$H4\",\"todo\":{\"index\":0,\"text\":\"otra cosa\",\"done\":false}}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> ...pero no si el texto citado ya no coincide" "$CODE" 400

BODY="{\"base\":\"$H4\",\"todo\":{\"index\":0,\"done\":false}}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> una escritura que no cambia nada responde 200" "$CODE" 200

BODY="{\"base\":\"$H4\",\"content\":\"x\",\"edits\":[{\"old\":\"a\",\"new\":\"b\"}]}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> dos formas a la vez se rechazan" "$CODE" 400
BODY="{\"base\":\"$H4\"}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> ninguna forma también" "$CODE" 400

# erin: recién creada, sin membresías y sin promover. dave YA es lead global a
# estas alturas — lo promovió carol arriba — así que no sirve para esta prueba.
ERIN=$(mkuser erin@bubble.test Erin | j "['id']")
ER=$(login erin@bubble.test)
echo
chk ">>> erin (sin membresía) no alcanza el documento" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$GETDOC" -H "Authorization: $ER")" 404
BODY='{"content":"x","base":""}'
CODE=$(patchdoc "$ER" "$BODY")
chk ">>> erin tampoco puede escribirlo" "$CODE" 404
chk "anónimo tampoco" "$(curl -s -o /dev/null -w '%{http_code}' "$GETDOC")" 401
chk "carol (lead global) sí lo lee sin ser miembro" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$GETDOC" -H "Authorization: $C")" 200

echo
NCOM=$(curl -s "$API/api/threads/$T1ID/history" -H "Authorization: $A" | python3 -c 'import sys,json
d=json.load(sys.stdin).get("commits") or []; print(len(d))')
# 4 escrituras que cambiaron algo: la primera, la del base correcto, el parche
# por contexto y el desmarcado. La que no cambió nada NO cuenta.
chk ">>> cada escritura que cambia algo es un commit (4)" "$NCOM" 4
WHO=$(curl -s "$API/api/threads/$T1ID/history" -H "Authorization: $A" | python3 -c 'import sys,json
c=json.load(sys.stdin).get("commits") or [""]; print("si" if "alice@bubble.test" in c[0] else "no")')
chk ">>> y queda firmado por quien escribió" "$WHO" si

curl -s -X PATCH "$API/api/collections/threads/records/$T1ID" -H "Authorization: $A" -H "$JS" \
  -d '{"name":"Renombrado"}' >/dev/null
TH2=$(curl -s "$API/api/collections/threads/records/$T1ID" -H "Authorization: $SU")
chk ">>> al renombrar, el archivo se mueve con el thread" \
  "$(echo "$TH2" | j "['doc_path']")" "threads/1-renombrado.md"
chk "...el contenido sobrevive la mudanza" "$(getdoc "$A" | j "['content']" | head -c 5)" "# Uno"
NC2=$(curl -s "$API/api/threads/$T1ID/history" -H "Authorization: $A" | python3 -c 'import sys,json
c=json.load(sys.stdin).get("commits") or []; print("si" if len(c)>=3 else "no")')
chk ">>> ...y la historia la sigue (git mv, no copiar y borrar)" "$NC2" si


# ------------------------------------- la wiki: docs/ y el árbol del workspace ----
echo
WSDOC="$API/api/workspaces/$ALPHA/document"
wpatch(){ curl -s -o /tmp/bubble-put.json -w '%{http_code}' -X PATCH "$WSDOC" \
  -H "Authorization: $1" -H "$JS" -d "$2"; }
E0=$(python3 -c 'import hashlib;print(hashlib.sha256(b"").hexdigest()[:32])')

BODY="{\"path\":\"docs/onboarding.md\",\"base\":\"$E0\",\"content\":\"# Onboarding\\n\"}"
CODE=$(wpatch "$A" "$BODY")
chk ">>> una página de docs/ nace sin crear ningún thread" "$CODE" 200
BODY="{\"path\":\"docs/guias/estilo.md\",\"base\":\"$E0\",\"content\":\"# Estilo\\n\"}"
CODE=$(wpatch "$A" "$BODY")
chk ">>> docs/ anida" "$CODE" 200
BODY="{\"path\":\"README.md\",\"base\":\"$E0\",\"content\":\"# Alpha\\n\"}"
CODE=$(wpatch "$A" "$BODY")
chk "README.md en la raíz" "$CODE" 200
BODY="{\"path\":\"docs/diagrama.excalidraw\",\"base\":\"$E0\",\"content\":\"{}\"}"
CODE=$(wpatch "$A" "$BODY")
chk "excalidraw se acepta" "$CODE" 200

BODY="{\"path\":\"threads/sub/anidado.md\",\"base\":\"$E0\",\"content\":\"x\"}"
CODE=$(wpatch "$A" "$BODY")
chk ">>> threads/ NO anida" "$CODE" 400
BODY="{\"path\":\"suelto.md\",\"base\":\"$E0\",\"content\":\"x\"}"
CODE=$(wpatch "$A" "$BODY")
chk ">>> en la raíz solo va README" "$CODE" 400
BODY="{\"path\":\"docs/foto.png\",\"base\":\"$E0\",\"content\":\"x\"}"
CODE=$(wpatch "$A" "$BODY")
chk ">>> un binario se rechaza (quiere otra puerta)" "$CODE" 400
BODY="{\"path\":\"../fuera.md\",\"base\":\"$E0\",\"content\":\"x\"}"
CODE=$(wpatch "$A" "$BODY")
chk ">>> una ruta que se escapa se rechaza" "$CODE" 400

chk "un miembro raso también escribe la wiki" \
  "$(wpatch "$B" "{\"path\":\"docs/de-bob.md\",\"base\":\"$E0\",\"content\":\"# Bob\\n\"}")" 200
chk ">>> erin (sin membresía) no escribe la wiki" \
  "$(wpatch "$ER" "{\"path\":\"docs/colado.md\",\"base\":\"$E0\",\"content\":\"x\"}")" 404

TREE=$(curl -s "$API/api/workspaces/$ALPHA/tree" -H "Authorization: $A")
chk ">>> el árbol lista docs, threads y README" \
  "$(echo "$TREE" | python3 -c 'import sys,json
p={e["path"] for e in json.load(sys.stdin)["entries"]}
need={"README.md","threads","docs","docs/onboarding.md","docs/guias","docs/guias/estilo.md"}
print("si" if need <= p else "faltan "+str(need-p))')" si
chk "...el título de un archivo es su nombre" \
  "$(echo "$TREE" | python3 -c 'import sys,json
print(next(e["title"] for e in json.load(sys.stdin)["entries"] if e["path"]=="docs/onboarding.md"))')" onboarding
chk "...y .git no se filtra" \
  "$(echo "$TREE" | python3 -c 'import sys,json
print("si" if not any(e["path"].startswith(".git") for e in json.load(sys.stdin)["entries"]) else "no")')" si
chk "erin no ve el árbol" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/workspaces/$ALPHA/tree" -H "Authorization: $ER")" 404

chk "borrar una página de docs/" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$WSDOC?path=docs/de-bob.md" -H "Authorization: $A")" 200
chk ">>> pero no el documento de un thread por esa puerta" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$WSDOC?path=threads/1-renombrado.md" -H "Authorization: $A")" 400

chk "el repo del workspace es un git de verdad" "$([ -d "$R/alpha/.git" ] && echo si || echo no)" si
chk "y el árbol queda limpio tras las escrituras" \
  "$(cd "$R/alpha" && git status --porcelain | wc -l | tr -d ' ')" 0


# ---------------------------------------------------- la evidencia (fase 2) ----
echo
EV(){ curl -s "$API/api/collections/events/records?perPage=200&filter=$1" -H "Authorization: $2"; }
evcount(){ EV "$1" "$SU" | j "['totalItems']"; }

chk ">>> crear un thread deja evidencia" \
  "$(evcount "(target='$T1ID'%26%26kind='thread-created')")" 1
chk ">>> cada escritura que cambió algo dejó evidencia (4)" \
  "$(evcount "(target='$T1ID'%26%26kind='document-changed')")" 4
chk ">>> la escritura que NO cambió nada no dejó evidencia" \
  "$(evcount "(target='$T1ID'%26%26kind='document-changed')")" 4
chk ">>> un link deja evidencia" \
  "$(evcount "(target='$T1ID'%26%26kind='link-added')")" 1
chk ">>> un comentario también, pero es pulso" \
  "$(evcount "(target='$T1ID'%26%26kind='comment')")" 1
chk ">>> la wiki se registra a grano de workspace" \
  "$([ "$(evcount "(kind='doc-changed')")" -ge 4 ] && echo si || echo no)" si
chk ">>> ...y NUNCA a grano de thread" \
  "$(evcount "(kind='doc-changed'%26%26target_type='thread')")" 0

chk "la evidencia queda firmada por quien la produjo" \
  "$(EV "(target='$T1ID'%26%26kind='thread-created')" "$SU" | j "['items'][0]['actor']")" "$AID"

# completar un thread es una TRANSICIÓN de estado, no un campo
curl -s -X PATCH "$API/api/collections/threads/records/$T1ID" -H "Authorization: $A" -H "$JS" \
  -d "{\"state\":\"$ST_A\"}" >/dev/null
chk "pasar a un estado que no es completed no completa nada" \
  "$(evcount "(target='$T1ID'%26%26kind='thread-completed')")" 0
# carol, no alice: alice quedó degradada a member arriba y crear estados es de lead.
ST_DONE=$(post states "$C" "{\"workspace\":\"$ALPHA\",\"name\":\"Hecho\",\"group\":\"completed\"}" | j "['id']")
curl -s -X PATCH "$API/api/collections/threads/records/$T1ID" -H "Authorization: $A" -H "$JS" \
  -d "{\"state\":\"$ST_DONE\"}" >/dev/null
chk ">>> llegar a un estado completed sí" \
  "$(evcount "(target='$T1ID'%26%26kind='thread-completed')")" 1
curl -s -X PATCH "$API/api/collections/threads/records/$T1ID" -H "Authorization: $A" -H "$JS" \
  -d '{"name":"Renombrado otra vez"}' >/dev/null
chk ">>> guardar un thread ya completado no lo completa de nuevo" \
  "$(evcount "(target='$T1ID'%26%26kind='thread-completed')")" 1

echo
chk ">>> nadie escribe evidencia desde un cliente" \
  "$(pcode events "$A" "{\"workspace\":\"$ALPHA\",\"target_type\":\"thread\",\"target\":\"$T1ID\",\"kind\":\"document-changed\",\"at\":\"2026-01-01 00:00:00.000Z\"}")" 400
chk ">>> ni el lead global" \
  "$(pcode events "$C" "{\"workspace\":\"$ALPHA\",\"target_type\":\"thread\",\"target\":\"$T1ID\",\"kind\":\"document-changed\",\"at\":\"2026-01-01 00:00:00.000Z\"}")" 400
chk "erin no ve la evidencia de alpha" \
  "$(EV "(workspace='$ALPHA')" "$ER" | j "['totalItems']")" 0
chk "un miembro sí la ve" \
  "$([ "$(EV "(workspace='$ALPHA')" "$A" | j "['totalItems']")" -gt 5 ] && echo si || echo no)" si


# --------------------------------------- los dos ejes derivados (fase 2) ----
echo
PRI(){ curl -s "$API/api/collections/thread_priority/records?perPage=100&filter=$1" -H "Authorization: $2"; }
T5=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Critico\",\"impact\":\"high\",\"urgency\":\"high\"}" | j "['id']")
T6=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Backlog\",\"impact\":\"low\",\"urgency\":\"low\"}" | j "['id']")
T7=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Medio\",\"impact\":\"mid\",\"urgency\":\"high\"}" | j "['id']")
chk ">>> prioridad derivada: alto x alto = P1" "$(PRI "(id='$T5')" "$A" | j "['items'][0]['priority']")" P1
chk ">>> bajo x bajo = P4" "$(PRI "(id='$T6')" "$A" | j "['items'][0]['priority']")" P4
chk ">>> medio x alto = P2" "$(PRI "(id='$T7')" "$A" | j "['items'][0]['priority']")" P2
# T2 nació sin impact ni urgency; T1 sí los trae desde arriba.
T2ID=$(echo "$T2" | j "['id']")
chk ">>> sin impacto ni urgencia, sin prioridad" "$(PRI "(id='$T2ID')" "$A" | j "['items'][0]['priority']")" ""
chk ">>> priority NO es una columna de threads" \
  "$(curl -s "$API/api/collections/threads/records/$T5" -H "Authorization: $SU" | python3 -c 'import sys,json;print("si" if "priority" not in json.load(sys.stdin) else "NO, es columna")')" si
# dos: el "Primer thread" del principio y el "Critico" de aquí.
chk "se puede filtrar por prioridad como cualquier campo" \
  "$(PRI "(priority='P1')" "$A" | j "['totalItems']")" 2
chk "erin no ve prioridades de alpha" "$(PRI "(workspace='$ALPHA')" "$ER" | j "['totalItems']")" 0

echo
BOARD=$(curl -s "$API/api/workspaces/$ALPHA/board" -H "Authorization: $A")
chk ">>> el board calcula heat sin guardarlo" \
  "$(echo "$BOARD" | python3 -c 'import sys,json
d=json.load(sys.stdin); print("si" if d["threads"] and "lifecycle" in d["threads"][0]["heat"] else "no")')" si
chk ">>> un thread completado sale closed, no caliente" \
  "$(echo "$BOARD" | python3 -c 'import sys,json
d=json.load(sys.stdin)
print(next(t["heat"]["lifecycle"] for t in d["threads"] if t["id"]=="'"$T1ID"'"))')" closed
chk ">>> un thread recién nacido sin evidencia NO está dormant" \
  "$(echo "$BOARD" | python3 -c 'import sys,json
d=json.load(sys.stdin)
print(next(t["heat"]["lifecycle"] for t in d["threads"] if t["id"]=="'"$T5"'"))')" hot
chk ">>> el board trae las dos escalas por separado" \
  "$(echo "$BOARD" | python3 -c 'import sys,json
d=json.load(sys.stdin)
t=next(t for t in d["threads"] if t["id"]=="'"$T5"'")
print("si" if t["priority"]=="P1" and t["heat"]["lifecycle"]=="hot" else t)')" si
chk "el board dice contra qué calibración clasificó" \
  "$(echo "$BOARD" | python3 -c 'import sys,json;print(int(json.load(sys.stdin)["tuning"]["CycleHours"]))')" 168
chk "erin no ve el board" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/workspaces/$ALPHA/board" -H "Authorization: $ER")" 404

echo
chk ">>> recalibrar cambia el veredicto sin migración ni backfill" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$API/api/collections/tuning/records/$(curl -s "$API/api/collections/tuning/records" -H "Authorization: $C" | j "['items'][0]['id']")" \
     -H "Authorization: $C" -H "$JS" -d '{"cycle_hours":1}')" 200
chk "...y un miembro raso no puede recalibrar" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$API/api/collections/tuning/records/$(curl -s "$API/api/collections/tuning/records" -H "Authorization: $C" | j "['items'][0]['id']")" \
     -H "Authorization: $B" -H "$JS" -d '{"cycle_hours":72}')" 404


# --------------------------------------------------- el MCP (fase 3) ----
echo
# El transporte es SSE: `event: message` y luego `data: {json}`. Se extrae el
# último objeto JSON de la respuesta.
mcp(){ # $1 token, $2 method, $3 params-json -> imprime el result como json
  curl -s -X POST "$API/mcp" -H "Authorization: $1" -H "$JS" \
    -H 'Accept: application/json, text/event-stream' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$2\",\"params\":$3}" \
  | python3 -c 'import sys,json,re
raw=sys.stdin.read()
objs=re.findall(r"^data: (.*)$", raw, re.M) or [raw]
try: print(json.dumps(json.loads(objs[-1])))
except Exception: print("{}")'
}
mcptool(){ mcp "$1" "tools/call" "{\"name\":\"$2\",\"arguments\":$3}"; }
# el texto que devuelve una tool
mcptext(){ mcptool "$1" "$2" "$3" | python3 -c 'import sys,json
d=json.load(sys.stdin)
r=d.get("result",{})
c=r.get("content") or []
print(c[0]["text"] if c else json.dumps(d))'; }

chk ">>> tools/list expone la superficie" \
  "$(mcp "$A" "tools/list" "{}" | python3 -c 'import sys,json
n=sorted(t["name"] for t in json.load(sys.stdin)["result"]["tools"])
print(",".join(n))')" \
  "board,complete_thread,create_thread,edit,guide,link,read,search,tree,workspaces"
chk ">>> la guía es prompt Y tool (no todo cliente lista prompts)" \
  "$(mcp "$A" "prompts/list" "{}" | python3 -c 'import sys,json
print(",".join(p["name"] for p in json.load(sys.stdin)["result"]["prompts"]))')" bubble-work
chk "la tool guide devuelve el mismo markdown que /api/guide" \
  "$(mcptext "$A" guide '{}' | head -1)" "# Bubble Work"
chk "sin token, /mcp no responde" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/mcp" -H "$JS" -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}')" 401

chk ">>> workspaces respeta la frontera: erin no ve ninguno" \
  "$(mcptext "$ER" workspaces '{}' | python3 -c 'import sys,json
try: print(len(json.loads(sys.stdin.read()) or []))
except Exception: print(0)')" 0

# ---- el bucle completo de un agente, por slug ----
echo
NT=$(mcptext "$A" create_thread '{"workspace":"alpha","name":"Trabajo del agente","impact":"high","urgency":"high"}')
NTID=$(echo "$NT" | j "['id']")
chk ">>> un agente crea un thread por SLUG, sin conocer ids" "$([ -n "$NTID" ] && echo si || echo no)" si
chk "...y el server le puso número y ruta" "$(echo "$NT" | j "['doc_path']")" "threads/$(echo "$NT" | j "['seq']")-trabajo-del-agente.md"

RD=$(mcptext "$A" read "{\"thread\":\"$NTID\"}")
H=$(echo "$RD" | j "['hash']")
chk "read devuelve el hash que la escritura necesita" "$([ -n "$H" ] && echo si || echo no)" si
W1=$(mcptext "$A" edit "{\"thread\":\"$NTID\",\"base\":\"$H\",\"content\":\"# Trabajo\\n\\n- [ ] investigar el rate limit\\n\"}")
chk ">>> escribe el documento" "$(echo "$W1" | j "['done']")" 0
H2=$(echo "$W1" | j "['hash']")
chk ">>> escribir con el base viejo se rechaza también por MCP" \
  "$(mcptext "$A" edit "{\"thread\":\"$NTID\",\"base\":\"$H\",\"content\":\"pisado\"}" | grep -c "changed since you read it")" 1
W2=$(mcptext "$A" edit "{\"thread\":\"$NTID\",\"base\":\"$H2\",\"todo\":{\"index\":0,\"done\":true}}")
chk ">>> marca la casilla y devuelve el conteo" "$(echo "$W2" | j "['done']")" 1

chk ">>> search encuentra lo que acaba de escribir" \
  "$(mcptext "$A" search '{"workspace":"alpha","query":"rate limit"}' | python3 -c 'import sys,json
h=json.loads(sys.stdin.read()) or []
print("si" if any("trabajo-del-agente" in x["path"] for x in h) else "no")')" si
chk ">>> link: evidencia externa" \
  "$(mcptext "$A" link "{\"thread\":\"$NTID\",\"url\":\"https://example.com/pr/9\",\"title\":\"PR\"}" | j "['url']")" "https://example.com/pr/9"

chk ">>> y la burbuja se calienta por eso: el thread sale hot" \
  "$(mcptext "$A" board '{"workspace":"alpha"}' | python3 -c 'import sys,json
d=json.loads(sys.stdin.read())
print(next(t["heat"]["lifecycle"] for t in d["threads"] if t["id"]=="'"$NTID"'"))')" hot
chk "...con su prioridad derivada al lado, sin mezclarse" \
  "$(mcptext "$A" board '{"workspace":"alpha"}' | python3 -c 'import sys,json
d=json.loads(sys.stdin.read())
print(next(t["priority"] for t in d["threads"] if t["id"]=="'"$NTID"'"))')" P1
chk ">>> completar es un cambio de estado, no un examen" \
  "$(mcptext "$A" complete_thread "{\"thread\":\"$NTID\"}" | j "['id']")" "$NTID"
chk "...y el board lo refleja" \
  "$(mcptext "$A" board '{"workspace":"alpha"}' | python3 -c 'import sys,json
d=json.loads(sys.stdin.read())
print(next(t["heat"]["lifecycle"] for t in d["threads"] if t["id"]=="'"$NTID"'"))')" closed

chk ">>> erin no alcanza el thread por MCP" \
  "$(mcptext "$ER" read "{\"thread\":\"$NTID\"}" | grep -c "not found")" 1


# ------------------------------------------------------------ imágenes ----
echo
PNG=/tmp/bubble-test.png
printf '\x89PNG\r\n\x1a\n' > "$PNG"; head -c 300 /dev/urandom >> "$PNG"
UP=$(curl -s -X POST "$API/api/workspaces/$ALPHA/asset" -H "Authorization: $A" \
  -F "file=@$PNG" -F "name=Mi Diagrama.png")
chk ">>> se sube una imagen y aterriza en assets/" "$(echo "$UP" | j "['path']")" "assets/mi-diagrama.png"
chk "...y devuelve la url para embeberla" \
  "$(echo "$UP" | j "['url']")" "/api/workspaces/$ALPHA/file?path=assets/mi-diagrama.png"

FILEURL="$API/api/workspaces/$ALPHA/file?path=assets/mi-diagrama.png"
chk ">>> se sirve con su content-type" \
  "$(curl -s -o /dev/null -w '%{content_type}' "$FILEURL" -H "Authorization: $A")" "image/png"
chk ">>> y con la política que impide que un svg ejecute algo" \
  "$(curl -s -D - -o /dev/null "$FILEURL" -H "Authorization: $A" | grep -ci "content-security-policy: default-src 'none'; sandbox")" 1
chk "...y con nosniff" \
  "$(curl -s -D - -o /dev/null "$FILEURL" -H "Authorization: $A" | grep -ci "x-content-type-options: nosniff")" 1
chk ">>> los bytes vuelven idénticos" \
  "$(curl -s "$FILEURL" -H "Authorization: $A" | cmp -s - "$PNG" && echo si || echo no)" si

chk ">>> una imagen es tan privada como la escritura: erin no la ve" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$FILEURL" -H "Authorization: $ER")" 404
chk "anónimo tampoco" "$(curl -s -o /dev/null -w '%{http_code}' "$FILEURL")" 401

chk ">>> en assets/ no entra un documento" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/workspaces/$ALPHA/asset" \
     -H "Authorization: $A" -F "file=@$PNG" -F "name=notas.md")" 400
BIG=/tmp/bubble-big.png; head -c 200000 /dev/urandom > "$BIG"
chk "una imagen dentro del límite pasa" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/workspaces/$ALPHA/asset" \
     -H "Authorization: $A" -F "file=@$BIG" -F "name=grande.png")" 200
chk ">>> el árbol lista la imagen" \
  "$(curl -s "$API/api/workspaces/$ALPHA/tree" -H "Authorization: $A" | python3 -c 'import sys,json
e=json.load(sys.stdin)["entries"]
print(next((x["area"] for x in e if x["path"]=="assets/mi-diagrama.png"), "falta"))')" asset
chk "y quedó commiteada como todo lo demás" \
  "$(cd "$R/alpha" && git log --oneline -- assets/mi-diagrama.png | wc -l | tr -d ' ')" 1
rm -f "$PNG" "$BIG"


# --------------------------------- el superuser: opera la caja, no trabaja ----
echo
chk ">>> el superuser SÍ ve el board (se salta las reglas en todos lados o en ninguno)" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/workspaces/$ALPHA/board" -H "Authorization: $SU")" 200
chk ">>> y el documento de un thread" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$GETDOC" -H "Authorization: $SU")" 200
SUH=$(getdoc "$SU" | j "['hash']")
BODY="{\"base\":\"$SUH\",\"content\":\"escrito por nadie\"}"
CODE=$(patchdoc "$SU" "$BODY")
chk ">>> pero NO escribe: no hay a quién atribuirle la escritura" "$CODE" 400
chk "...y lo dice, en vez de un 404 misterioso" \
  "$(python3 -c 'import json;print("si" if "attributed to a person" in json.load(open("/tmp/bubble-put.json"))["message"] else "no")')" si
chk "el contenido quedó intacto" "$(getdoc "$A" | j "['content']" | head -c 5)" "# Uno"

# la misma persona puede tener las dos cuentas, con el mismo correo
chk ">>> el mismo correo existe en users y en _superusers a la vez" \
  "$(pcode users "$SU" '{"email":"root@bubble.test","password":"passwordpass","passwordConfirm":"passwordpass","role":"member","verified":true}')" 200
chk "...y autentica como persona" \
  "$(curl -s -X POST "$API/api/collections/users/auth-with-password" -H "$JS" \
     -d '{"identity":"root@bubble.test","password":"passwordpass"}' | j "['record']['email']")" "root@bubble.test"
chk "...sin dejar de autenticar como superuser" \
  "$(curl -s -X POST "$API/api/collections/_superusers/auth-with-password" -H "$JS" \
     -d '{"identity":"root@bubble.test","password":"rootrootroot"}' | j "['record']['email']")" "root@bubble.test"

echo; echo "  $pass pasaron, $fail fallaron"
[ "$fail" = "0" ]
