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
go test ./... || exit 1

echo "==> live"
D=${TMPDIR:-/tmp}/bubble-test-data
B=./dist/bubble
API=http://127.0.0.1:8099
pkill -f "bubble serve" 2>/dev/null; sleep 0.5; rm -rf "$D"
[[ -x "$B" ]] || scripts/build.sh

pass=0; fail=0
ok(){ echo "  PASS  $1"; pass=$((pass+1)); }
no(){ echo "  FAIL  $1  -> $2"; fail=$((fail+1)); }
chk(){ if [ "$2" = "$3" ]; then ok "$1"; else no "$1" "esperaba $3, dio $2"; fi; }

"$B" superuser upsert root@bubble.test rootrootroot --dir "$D" >/dev/null 2>&1
"$B" serve --http 127.0.0.1:8099 --dir "$D" >/tmp/bubble-test.log 2>&1 &
trap 'pkill -f "bubble serve" 2>/dev/null' EXIT
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

echo; echo "  $pass pasaron, $fail fallaron"
[ "$fail" = "0" ]
