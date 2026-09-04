#!/usr/bin/env bash
# Every test: the Go ones, then the ones that need a running server.
#
#   just test
#
# The second half is here because an API rule is not a function you can call. The
# rule that matters is `isLead`: two `?=` conditions on the same back-relation. If
# they bound to DIFFERENT membership rows, any member of a workspace that has any
# lead could edit it. The three >>> lines prove they do not, against a real
# server, on a throwaway data directory.
#
# These assertions want to be Go tests eventually — PocketBase ships a `tests`
# package for exactly this. Curl is what they are until somebody moves them.
set -uo pipefail

echo "==> go test"
go test ./... || exit 1

echo "==> live"
[[ -x ./dist/bubble ]] || scripts/build.sh
D=${TMPDIR:-/tmp}/bubble-test-data
B=./dist/bubble
API=http://127.0.0.1:8099
pkill -f "bubble serve" 2>/dev/null; sleep 0.5; rm -rf "$D"
pass=0; fail=0
ok(){ echo "  PASS  $1"; pass=$((pass+1)); }
no(){ echo "  FAIL  $1  -> $2"; fail=$((fail+1)); }
chk(){ if [ "$2" = "$3" ]; then ok "$1"; else no "$1" "esperaba $3, dio $2"; fi; }

"$B" superuser upsert root@bubble.test rootrootroot --dir "$D" >/dev/null 2>&1
"$B" serve --http 127.0.0.1:8099 --dir "$D" >/tmp/bubble-test.log 2>&1 &
trap 'pkill -f "bubble serve" 2>/dev/null' EXIT
for i in $(seq 1 60); do curl -sf "$API/api/health" >/dev/null 2>&1 && break; sleep 0.25; done

j(){ python3 -c 'import sys,json
try:
    d=json.load(sys.stdin); exec("v=d"+sys.argv[1]); print(v)
except Exception: print("")' "$1"; }
code(){ curl -s -o /dev/null -w '%{http_code}' "$@"; }
JS='Content-Type: application/json'

SU=$(curl -s -X POST "$API/api/collections/_superusers/auth-with-password" -H "$JS" \
  -d '{"identity":"root@bubble.test","password":"rootrootroot"}' | j "['token']")
[ -n "$SU" ] && ok "superuser autentica" || { no "superuser autentica" "sin token"; exit 1; }

mkuser(){ curl -s -X POST "$API/api/collections/users/records" -H "Authorization: $SU" -H "$JS" \
  -d "{\"email\":\"$1\",\"password\":\"passwordpass\",\"passwordConfirm\":\"passwordpass\",\"display_name\":\"$2\",\"verified\":true}"; }
login(){ curl -s -X POST "$API/api/collections/users/auth-with-password" -H "$JS" \
  -d "{\"identity\":\"$1\",\"password\":\"passwordpass\"}" | j "['token']"; }

AID=$(mkuser alice@bubble.test Alice | j "['id']")
BID=$(mkuser bob@bubble.test Bob     | j "['id']")
if [ -n "$AID" ] && [ -n "$BID" ]; then ok "users acepta display_name"; else no "crear users" "$AID/$BID"; fi

A=$(login alice@bubble.test); B=$(login bob@bubble.test)
if [ -n "$A" ] && [ -n "$B" ]; then ok "alice y bob autentican"; else no "login" "vacío"; fi

mkws(){ curl -s -X POST "$API/api/collections/workspaces/records" -H "Authorization: $1" -H "$JS" \
  -d "{\"name\":\"$2\",\"slug\":\"$3\"}"; }
ALPHA=$(mkws "$A" Alpha alpha | j "['id']")
BETA=$(mkws  "$B" Beta  beta  | j "['id']")
if [ -n "$ALPHA" ] && [ -n "$BETA" ]; then ok "alice y bob crean workspace"; else no "crear workspace" "$ALPHA/$BETA"; fi

MS=$(curl -s "$API/api/collections/memberships/records?filter=(workspace='$ALPHA')" -H "Authorization: $SU")
if [ "$(echo "$MS"|j "['totalItems']")" = "1" ] && [ "$(echo "$MS"|j "['items'][0]['role']")" = "lead" ] \
   && [ "$(echo "$MS"|j "['items'][0]['user']")" = "$AID" ]; then
  ok "membresía fundadora: 1 fila, role=lead, user=el creador"
else no "membresía fundadora" "$(echo "$MS"|head -c 200)"; fi

WA=$(curl -s "$API/api/collections/workspaces/records" -H "Authorization: $A")
WB=$(curl -s "$API/api/collections/workspaces/records" -H "Authorization: $B")
if [ "$(echo "$WA"|j "['totalItems']")" = "1" ] && [ "$(echo "$WA"|j "['items'][0]['slug']")" = "alpha" ] \
   && [ "$(echo "$WB"|j "['items'][0]['slug']")" = "beta" ]; then
  ok "cada quien lista SOLO su workspace"
else no "aislamiento de lista" "A=$(echo "$WA"|j "['totalItems']") B=$(echo "$WB"|j "['totalItems']")"; fi

chk "alice no ve beta por id" "$(code "$API/api/collections/workspaces/records/$BETA" -H "Authorization: $A")" 404
chk "anónimo lista 0 workspaces" "$(curl -s "$API/api/collections/workspaces/records" | j "['totalItems']")" 0

curl -s -X POST "$API/api/collections/memberships/records" -H "Authorization: $SU" -H "$JS" \
  -d "{\"workspace\":\"$ALPHA\",\"user\":\"$BID\",\"role\":\"member\"}" >/dev/null
chk "bob (member) ahora ve alpha y beta" \
  "$(curl -s "$API/api/collections/workspaces/records" -H "Authorization: $B" | j "['totalItems']")" 2

echo
chk ">>> bob (member) NO puede editar alpha  [las dos ?= atan a la MISMA fila]" \
  "$(code -X PATCH "$API/api/collections/workspaces/records/$ALPHA" -H "Authorization: $B" -H "$JS" -d '{"name":"Secuestrado"}')" 404
chk ">>> bob (member) NO puede borrar alpha" \
  "$(code -X DELETE "$API/api/collections/workspaces/records/$ALPHA" -H "Authorization: $B")" 404
chk ">>> alice (lead) SÍ puede editar alpha" \
  "$(code -X PATCH "$API/api/collections/workspaces/records/$ALPHA" -H "Authorization: $A" -H "$JS" -d '{"name":"Alpha renombrado"}')" 200
echo

chk "alice ve el roster de alpha (2), no el de beta" \
  "$(curl -s "$API/api/collections/memberships/records" -H "Authorization: $A" | j "['totalItems']")" 2
chk "ni el lead crea membresías desde el cliente" \
  "$(code -X POST "$API/api/collections/memberships/records" -H "Authorization: $A" -H "$JS" \
     -d "{\"workspace\":\"$ALPHA\",\"user\":\"$BID\",\"role\":\"lead\"}")" 400
chk "membresía duplicada rechazada (índice único)" \
  "$(code -X POST "$API/api/collections/memberships/records" -H "Authorization: $SU" -H "$JS" \
     -d "{\"workspace\":\"$ALPHA\",\"user\":\"$BID\",\"role\":\"lead\"}")" 400
chk "slug duplicado rechazado" \
  "$(code -X POST "$API/api/collections/workspaces/records" -H "Authorization: $A" -H "$JS" -d '{"name":"Otro","slug":"alpha"}')" 400
chk "slug con espacios/mayúsculas rechazado" \
  "$(code -X POST "$API/api/collections/workspaces/records" -H "Authorization: $A" -H "$JS" -d '{"name":"Otro","slug":"Con Espacios"}')" 400

echo; echo "  $pass pasaron, $fail fallaron"
[ "$fail" = "0" ]
