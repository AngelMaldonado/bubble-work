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
# El puerto, y con él el directorio de datos.
#
# Fijo bastaba mientras esto sólo corría en la máquina de quien escribe. En un
# runner compartido —una VM que atiende varios repositorios— dos suites con el
# mismo puerto se pisan, y el fallo parece del código. Quien comparta máquina
# pone `BUBBLE_TEST_PORT` y se lleva su propio puerto y su propio directorio.
PORT=${BUBBLE_TEST_PORT:-8099}
D=${TMPDIR:-/tmp}/bubble-test-data-$PORT
B=./dist/bubble
API=http://127.0.0.1:$PORT
R=${TMPDIR:-/tmp}/bubble-test-repos-$PORT

# Kill only OUR leftover, matched by the test port — never a bare
# `pkill -f "bubble serve"`.
#
# That pattern matches every server on the machine, including the one somebody is
# looking at in a browser. It ran at the start AND in the trap, so every
# `just check` took down a running `just dev` twice. The tests are supposed to be
# invisible to whoever is working.
pkill -f "bubble serve.*$PORT" 2>/dev/null; sleep 0.3
rm -rf "$D" "$R"
[[ -x "$B" ]] || scripts/build.sh

pass=0; fail=0
ok(){ echo "  PASS  $1"; pass=$((pass+1)); }
no(){ echo "  FAIL  $1  -> $2"; fail=$((fail+1)); }
chk(){ if [ "$2" = "$3" ]; then ok "$1"; else no "$1" "esperaba $3, dio $2"; fi; }

"$B" superuser upsert root@bubble.test rootrootroot --dir "$D" >/dev/null 2>&1
BUBBLE_REPOS="$R" "$B" serve --http 127.0.0.1:$PORT --dir "$D" >/tmp/bubble-test-$PORT.log 2>&1 &
SRV=$!
# By PID: the only process this script is entitled to end is the one it started.
trap 'kill "$SRV" 2>/dev/null' EXIT
for i in $(seq 1 60); do curl -sf "$API/api/health" >/dev/null 2>&1 && break; sleep 0.25; done
if ! curl -sf "$API/api/health" >/dev/null 2>&1; then
  echo "  el server no arrancó — ver /tmp/bubble-test-$PORT.log" >&2; exit 1
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
# El equipo que la migración crea para todo lo que ya existía.
TI=$(curl -s "$API/api/collections/teams/records" -H "Authorization: $SU" | j "['items'][0]['id']")
chk ">>> todo nace en un equipo: TI" "$(curl -s "$API/api/collections/teams/records" -H "Authorization: $SU" | j "['items'][0]['name']")" TI

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

# Reaplicar la MISMA contraseña no debe tocar nada: `SetPassword` rota el
# `tokenKey`, que es parte de lo que firma los tokens de esa persona, y
# `just dev` corre esto en cada arranque. Sin este cuidado, cada reinicio sacaba
# al operador de su navegador y mataba el token que estuviera usando su agente,
# sin que nada hubiera cambiado.
# `$B` ya no es el binario a esta altura del archivo — es el token de bob.
./dist/bubble person alice@bubble.test passwordpass member --dir "$D" >/dev/null 2>&1
chk ">>> reaplicar la misma contraseña NO invalida los tokens vivos" \
  "$(code "$API/api/collections/workspaces/records" -H "Authorization: $A")" 200

ALPHA=$(post workspaces "$A" '{"name":"Alpha","slug":"alpha"}' | j "['id']")
BETA=$(post  workspaces "$B" '{"name":"Beta","slug":"beta"}'   | j "['id']")
if [ -n "$ALPHA" ] && [ -n "$BETA" ]; then ok "alice y bob crean workspace"; else no "crear workspace" "$ALPHA/$BETA"; fi

# Un workspace recién fundado ya puede trabajar: sin estados, ningún thread
# puede completarse nunca, y "Terminar" no tiene a dónde apuntar.
WSS=$(curl -s "$API/api/collections/states/records?perPage=50&filter=(workspace='$ALPHA')" -H "Authorization: $A")
chk ">>> fundar un workspace le da su flujo de trabajo" "$(echo "$WSS" | j "['totalItems']")" 3
chk "...con uno por defecto" \
  "$(echo "$WSS" | python3 -c 'import sys,json;print(sum(1 for x in json.load(sys.stdin)["items"] if x["is_default"]))')" 1
chk "...y uno que SÍ completa" \
  "$(echo "$WSS" | python3 -c 'import sys,json;print(sum(1 for x in json.load(sys.stdin)["items"] if x["group"]=="completed"))')" 1

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
# Nombres propios: fundar el workspace ya dejó "Por hacer / En curso / Hecho",
# y el índice (workspace, name) es único.
ST_A=$(post states "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"En revisión\",\"group\":\"started\",\"position\":4}" | j "['id']")
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
# Lo dicho, dicho: el hilo es un registro al que se AGREGA. Ni el autor lo
# reescribe — un hilo donde las frases cambian deja de servir para entender por
# qué se decidió algo, y las respuestas quedan contestando a lo que ya no está.
# 403 y no 404, y la diferencia dice algo: un 404 es "no hay regla que te
# incluya" y esconde si la fila existe; un 403 es "esta puerta está cerrada para
# todos", que es exactamente lo que pasa aquí.
chk ">>> alice no puede editar el comentario de bob" \
  "$(code -X PATCH "$API/api/collections/comments/records/$C1ID" -H "Authorization: $A" -H "$JS" -d '{"body":"editado"}')" 403
chk ">>> ni bob el suyo: los comentarios se escriben, no se reescriben" \
  "$(code -X PATCH "$API/api/collections/comments/records/$C1ID" -H "Authorization: $B" -H "$JS" -d '{"body":"editado"}')" 403
chk ">>> y nadie los borra" \
  "$(code -X DELETE "$API/api/collections/comments/records/$C1ID" -H "Authorization: $B")" 403

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
# Leer y escribir responden la misma forma: la web redibuja con lo que devolvió
# la escritura, así que un campo que solo aparece en una de las dos se pierde.
chk "la lectura trae el html del servidor" "$(echo "$D1" | j "['html']" | head -c 7)" "<h1 id="
chk "...y la cuenta de casillas hechas" "$(echo "$D1" | j "['done']")" "0"
H1=$(echo "$D1" | j "['hash']")

BODY="{\"content\":\"pisado\",\"base\":\"$H0\"}"
CODE=$(patchdoc "$A" "$BODY")
chk ">>> escribir con un base viejo es CONFLICTO, no sobreescritura" "$CODE" 409
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

BODY="{\"path\":\"threads/uno/dos/tres.md\",\"base\":\"$E0\",\"content\":\"x\"}"
CODE=$(wpatch "$A" "$BODY")
chk ">>> threads/ anida UN nivel, no dos" "$CODE" 400
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

# Cuánto cambió, en líneas. No lo calcula nadie dos veces: cada escritura es un
# commit, así que git ya tiene la respuesta.
echo
CH=$(curl -s "$API/api/workspaces/$ALPHA/changes" -H "Authorization: $A")
chk ">>> el ciclo dice cuánto se escribió en cada archivo" \
  "$(echo "$CH" | python3 -c 'import sys,json
d=json.load(sys.stdin)["changes"]
c=next((x for x in d if x["path"].startswith("threads/")), None)
print("si" if c and c["added"] > 0 else d)')" si
chk ">>> ...y el diff de un documento viene del propio git" \
  "$(curl -s "$API/api/workspaces/$ALPHA/diff?path=docs/onboarding.md" -H "Authorization: $A" | python3 -c 'import sys,json
print("si" if "+# Onboarding" in json.load(sys.stdin)["diff"] else "no")')" si
chk ">>> ...y \"último cambio\" contesta aunque el ciclo esté vacío" \
  "$(curl -s "$API/api/workspaces/$ALPHA/diff?path=docs/onboarding.md&since=last" -H "Authorization: $A" | python3 -c 'import sys,json
print("si" if json.load(sys.stdin)["diff"] else "no")')" si
chk ">>> ...y trae los DOS lados, que es lo que una vista de diff necesita" \
  "$(curl -s "$API/api/workspaces/$ALPHA/diff?path=docs/onboarding.md&since=last" -H "Authorization: $A" | python3 -c 'import sys,json
d=json.load(sys.stdin)
print("si" if "after" in d and "before" in d and d["after"] else d)')" si
chk ">>> erin no ve lo que cambió en un workspace que no alcanza" \
  "$(code "$API/api/workspaces/$ALPHA/changes" -H "Authorization: $ER")" 404

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

# Un thread escribe más de un archivo cuando el trabajo lo pide: viven en SU
# carpeta, son suyos (la evidencia va al thread, no al workspace) y se borran
# como cualquier documento.
PG="threads/1-renombrado/notas.md"
BODY="{\"path\":\"$PG\",\"base\":\"$E0\",\"content\":\"# Notas\\n\"}"
chk ">>> un thread puede tener otro documento al lado" "$(wpatch "$A" "$BODY")" 200
chk ">>> ...y esa escritura calienta al THREAD, no al workspace" \
  "$(curl -s "$API/api/collections/events/records?perPage=1&sort=-at&filter=$(python3 -c "import urllib.parse;print(urllib.parse.quote(\"kind='document-changed'\"))")" \
     -H "Authorization: $SU" | j "['items'][0]['target_type']")" thread
chk ">>> ...y se borra como cualquier otro documento" \
  "$(code -X DELETE "$API/api/workspaces/$ALPHA/document?path=$PG" -H "Authorization: $A")" 200


# completar un thread es una TRANSICIÓN de estado, no un campo
curl -s -X PATCH "$API/api/collections/threads/records/$T1ID" -H "Authorization: $A" -H "$JS" \
  -d "{\"state\":\"$ST_A\"}" >/dev/null
chk "pasar a un estado que no es completed no completa nada" \
  "$(evcount "(target='$T1ID'%26%26kind='thread-completed')")" 0
# carol, no alice: alice quedó degradada a member arriba y crear estados es de lead.
ST_DONE=$(post states "$C" "{\"workspace\":\"$ALPHA\",\"name\":\"Cerrado\",\"group\":\"completed\"}" | j "['id']")
curl -s -X PATCH "$API/api/collections/threads/records/$T1ID" -H "Authorization: $A" -H "$JS" \
  -d "{\"state\":\"$ST_DONE\"}" >/dev/null
chk ">>> llegar a un estado completed sí" \
  "$(evcount "(target='$T1ID'%26%26kind='thread-completed')")" 1
curl -s -X PATCH "$API/api/collections/threads/records/$T1ID" -H "Authorization: $A" -H "$JS" \
  -d '{"name":"Renombrado otra vez"}' >/dev/null
chk ">>> guardar un thread ya completado no lo completa de nuevo" \
  "$(evcount "(target='$T1ID'%26%26kind='thread-completed')")" 1

echo
# El cuerpo en una VARIABLE, no dentro de "$( … )" como argumento de chk. Es la
# regla que este archivo ya se había puesto, y saltársela aquí costó cinco
# aserciones que pasaban en macOS y fallaban en el runner de CI: la MISMA llamada
# daba 400 en un sitio y 403 en el otro. Con el cuerpo en una variable da 403 en
# los dos, que es lo que corresponde — la regla de `events` es nil, y en
# PocketBase una regla nil significa "sólo superusers".
#
# Lo que se aprendió no es el mecanismo exacto del shell: es que una aserción que
# sólo dice "4xx" no distingue "la regla lo rechazó" de "la petición ni siquiera
# llegó a la regla", y verde por accidente es peor que ninguna aserción.
EVBODY="{\"workspace\":\"$ALPHA\",\"target_type\":\"thread\",\"target\":\"$T1ID\",\"kind\":\"document-changed\",\"at\":\"2026-01-01 00:00:00.000Z\"}"
EVCODE=$(pcode events "$A" "$EVBODY")
chk ">>> nadie escribe evidencia desde un cliente" "$EVCODE" 403
EVCODE=$(pcode events "$C" "$EVBODY")
chk ">>> ni el lead global" "$EVCODE" 403
chk "erin no ve la evidencia de alpha" \
  "$(EV "(workspace='$ALPHA')" "$ER" | j "['totalItems']")" 0
chk "un miembro sí la ve" \
  "$([ "$(EV "(workspace='$ALPHA')" "$A" | j "['totalItems']")" -gt 5 ] && echo si || echo no)" si


# --------------------------------------- los dos ejes derivados (fase 2) ----
echo
# La prioridad es de la BURBUJA. La decide quien orquesta, y quien ejecuta ya
# tiene su forma de ordenarse el día dentro de un cuerpo de trabajo.
PRI(){ curl -s "$API/api/collections/bubble_priority/records?perPage=100&filter=$1" -H "Authorization: $2"; }
B5=$(post bubbles "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Critica\",\"impact\":\"high\",\"urgency\":\"high\"}" | j "['id']")
B6=$(post bubbles "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Backlog\",\"impact\":\"low\",\"urgency\":\"low\"}" | j "['id']")
B7=$(post bubbles "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Media\",\"impact\":\"mid\",\"urgency\":\"high\"}" | j "['id']")
T5=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"bubble\":\"$B5\",\"name\":\"Critico\"}" | j "['id']")
chk ">>> prioridad derivada: alto x alto = P1" "$(PRI "(id='$B5')" "$A" | j "['items'][0]['priority']")" P1
chk ">>> bajo x bajo = P4" "$(PRI "(id='$B6')" "$A" | j "['items'][0]['priority']")" P4
chk ">>> medio x alto = P2" "$(PRI "(id='$B7')" "$A" | j "['items'][0]['priority']")" P2
chk ">>> sin impacto ni urgencia, sin prioridad" "$(PRI "(id='$BU_B')" "$B" | j "['items'][0]['priority']")" ""
chk ">>> priority NO es una columna de bubbles" \
  "$(curl -s "$API/api/collections/bubbles/records/$B5" -H "Authorization: $SU" | python3 -c 'import sys,json;print("si" if "priority" not in json.load(sys.stdin) else "NO, es columna")')" si
chk ">>> ...un thread no lleva impacto, urgencia ni objetivo: son de su burbuja" \
  "$(curl -s "$API/api/collections/threads/records/$T5" -H "Authorization: $SU" | python3 -c 'import sys,json;d=json.load(sys.stdin);print("si" if not any(k in d for k in ("impact","urgency","objective")) else sorted(d))')" si
chk "...pero sí su PROPIA prioridad, vacía hasta que el lead la pone" \
  "$(curl -s "$API/api/collections/threads/records/$T5" -H "Authorization: $SU" | j "['priority']")" ""
chk "se puede filtrar por prioridad como cualquier campo" \
  "$(PRI "(priority='P1')" "$A" | j "['totalItems']")" 1
chk "erin no ve prioridades de alpha" "$(PRI "(workspace='$ALPHA')" "$ER" | j "['totalItems']")" 0

# ----------------------------------------- el planeador: fase 5 (server) ----
#
# El planeador es del DEPARTAMENTO, no de un workspace. Los objetivos que
# aparecen cuando alguien los dice en voz alta —clientes, rentabilidad, ISO
# 9001— no son de un proyecto: los proyectos son lo que cuelga de ellos. Y una
# nota entra al inbox antes de que nadie sepa de qué proyecto es; decidirlo es
# justo lo que hace triar.
#
# Lo que NO hay aquí es una tarjeta. Un planeador con tarjetas propias es un
# segundo inventario del trabajo al lado de los threads. Las columnas son los
# `states` que el workspace ya define y lo que se mueve es un THREAD.
echo
OBJ=$(post objectives "$C" "{\"name\":\"Cerrar el trimestre en verde\",\"outcome\":\"todo el ingreso facturado\"}" | j "['id']")
chk ">>> el lead GLOBAL define el objetivo" "$([ -n "$OBJ" ] && echo si || echo no)" si
chk ">>> el lead de un workspace NO lo define: los objetivos son de los leads del equipo" \
  "$(pcode objectives "$B" "{\"name\":\"Mio\"}")" 403
chk "el mismo nombre dos veces se rechaza" \
  "$(pcode objectives "$C" "{\"name\":\"Cerrar el trimestre en verde\"}")" 400
chk ">>> el plan lo LEE el lead global, y nadie más" \
  "$(curl -s "$API/api/collections/objectives/records" -H "Authorization: $C" | j "['totalItems']")" 1
chk "...ni siquiera un lead de workspace" \
  "$(curl -s "$API/api/collections/objectives/records" -H "Authorization: $A" | j "['totalItems']")" 0
chk "...ni erin, que no es de ningún workspace" \
  "$(curl -s "$API/api/collections/objectives/records" -H "Authorization: $ER" | j "['totalItems']")" 0
chk "anónimo tampoco" \
  "$(curl -s "$API/api/collections/objectives/records" | j "['totalItems']")" 0

# Un objetivo cruza proyectos, y es lo que lo hace valer la pena. Ahora con
# burbujas: una burbuja es de un workspace, y el objetivo las junta.
BU_O1=$(post bubbles "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Bajo objetivo\",\"objective\":\"$OBJ\"}" | j "['id']")
BU_O2=$(post bubbles "$B" "{\"workspace\":\"$BETA\",\"name\":\"Beta bajo el mismo objetivo\",\"objective\":\"$OBJ\"}" | j "['id']")
chk ">>> burbujas de DOS workspaces cuelgan del mismo objetivo" \
  "$([ -n "$BU_O1" ] && [ -n "$BU_O2" ] && echo si || echo no)" si
chk "borrar el objetivo NO se lleva el trabajo hecho bajo él" \
  "$(code -X DELETE "$API/api/collections/objectives/records/$OBJ" -H "Authorization: $C")" 204
chk "...y la burbuja sigue ahí, sin objetivo" \
  "$(curl -s "$API/api/collections/bubbles/records/$BU_O1" -H "Authorization: $A" | j "['objective']")" ""

echo
IN1=$(post inbox_items "$B" "{\"note\":\"revisar el rate limit del portal\"}")
IN1ID=$(echo "$IN1" | j "['id']")
chk ">>> cualquiera captura en el inbox" "$([ -n "$IN1ID" ] && echo si || echo no)" si
chk ">>> y queda firmado por quien capturó, no por quien dijo" \
  "$(echo "$IN1" | j "['captured_by']")" "$BID"
chk ">>> capturar NO es evidencia: no calienta nada" \
  "$(evcount "(kind='inbox-captured')")" 0
chk ">>> anónimo no captura" \
  "$(code -X POST "$API/api/collections/inbox_items/records" -H "$JS" -d '{"note":"hola"}')" 400
SUPBODY="{\"note\":\"suplantada\",\"captured_by\":\"$BID\"}"
SUP=$(post inbox_items "$A" "$SUPBODY")
chk "nadie puede firmar una nota como otro" "$(echo "$SUP" | j "['captured_by']")" "$AID"

# Lo escrito en una nota se guarda. El panel dejaba editar el título y el texto,
# los cambiaba en el objeto de la lista y nadie los mandaba al servidor: recargar
# los borraba. Es de las peores formas de perder algo, porque mientras la
# pantalla está abierta parece guardado.
chk ">>> una nota se puede reescribir, y con cuerpo" \
  "$(code -X PATCH "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $B" -H "$JS" \
     -d '{"note":"revisar el rate limit del portal","body":"# Qué pidió\n\nQue no tire 429 en la hora pico."}')" 200
chk ">>> ...y vuelve entero" \
  "$(curl -s "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $B" | grep -c '429 en la hora pico')" 1

# El motor de los threads dibuja, y un bloque ```excalidraw es la escena entera
# en JSON: una docena de cajas ya pasa de 30000 caracteres. El tope viejo (20000)
# rechazaba el PATCH entero — título incluido — justo al pegar un diagrama.
BIGBODY=${TMPDIR:-/tmp}/nota-grande.json
python3 -c "import json,sys;json.dump({'body':'# Diagrama\n\n\u0060\u0060\u0060excalidraw\n'+('{\"type\":\"rectangle\"},'*3000)+'\n\u0060\u0060\u0060\n'},open(sys.argv[1],'w'))" "$BIGBODY"
chk ">>> un diagrama largo cabe en el cuerpo de una nota" \
  "$(code -X PATCH "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $B" -H "$JS" -d @"$BIGBODY")" 200
chk "...y vuelve entero" \
  "$(curl -s "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $B" | python3 -c "import json,sys;print(len(json.load(sys.stdin)['body'])>60000)")" True

# Los topes, dichos ANTES de guardar. La pantalla guarda sola —al dejar de
# escribir, al salir de un campo— y un 400 en ese momento no tenía dónde
# aparecer: lo escrito se perdía al recargar. Salen del esquema, no de una copia.
LIM=$(curl -s "$API/api/limits" -H "Authorization: $B")
chk ">>> los topes de cada campo se pueden preguntar" \
  "$(echo "$LIM" | j "['fields']['inbox_items.body']")" 2000000
chk "...cada uno el suyo" \
  "$(echo "$LIM" | j "['fields']['workspaces.name']")" 100
chk "...y el de un comentario, que se mide en bytes (5 MB)" \
  "$(echo "$LIM" | j "['fields']['comments.body']")" 5242880
chk "...con sesión" "$(code "$API/api/limits")" 401
# Qué se puede SUBIR, por la misma puerta. Publicado y no copiado en el cliente:
# un diálogo de archivos que filtra por una lista adivinada acaba tachando algo
# que el servidor sí acepta, y nadie se entera de que estaba permitido.
chk ">>> ...y qué archivos se admiten, para el diálogo de archivos" \
  "$(echo "$LIM" | python3 -c 'import sys,json;print(",".join(json.load(sys.stdin)["accept"]))')" \
  ".avif,.csv,.gif,.jpeg,.jpg,.json,.pdf,.png,.svg,.txt,.webp"
chk "...y el tope por archivo" "$(echo "$LIM" | j "['bytes']")" 5242880

# Una nota lleva imágenes, y como no tiene workspace van en el propio registro.
# `files+` AÑADE: sin el `+` cada pegado borraría la imagen anterior y la nota
# citaría archivos que ya no existen.
PNG1=${TMPDIR:-/tmp}/nota-1.png; PNG2=${TMPDIR:-/tmp}/nota-2.png
python3 -c "import base64,sys;open(sys.argv[1],'wb').write(base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=='))" "$PNG1"
cp "$PNG1" "$PNG2"
curl -s -o /dev/null -X PATCH "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $B" -F "files+=@$PNG1"
UP2=$(curl -s -X PATCH "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $B" -F "files+=@$PNG2")
chk ">>> una nota guarda las imágenes que se le pegan" \
  "$(echo "$UP2" | python3 -c 'import sys,json;print(len(json.load(sys.stdin).get("files",[])))')" 2
chk ">>> ...y otra persona no le pega nada" \
  "$(code -X PATCH "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $A" -F "files+=@$PNG1")" 404
# Un SVG es un documento que puede ejecutar código, y servido desde nuestro
# propio origen lo haría con la sesión de quien lo mira.
printf '<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>' > ${TMPDIR:-/tmp}/nota.svg
chk ">>> un SVG no entra: podría ejecutar código en nuestro origen" \
  "$(code -X PATCH "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $B" -F "files+=@${TMPDIR:-/tmp}/nota.svg;type=image/svg+xml")" 400

# Triar: la nota se convierte en un thread —y ahí se decide de qué workspace es—
# y se queda apuntando a lo que fue.
T9=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Rate limit del portal\"}" | j "['id']")
chk ">>> el lead global tría la nota a un thread" \
  "$(code -X PATCH "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $C" -H "$JS" -d "{\"thread\":\"$T9\"}")" 200
chk "...y la nota conserva a dónde fue" \
  "$(curl -s "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $C" | j "['thread']")" "$T9"

IN2ID=$(post inbox_items "$A" "{\"note\":\"la de alice\"}" | j "['id']")
chk ">>> quien la escribió la edita" \
  "$(code -X PATCH "$API/api/collections/inbox_items/records/$IN2ID" -H "Authorization: $A" -H "$JS" -d '{"note":"la de alice, corregida"}')" 200
chk ">>> otra persona NO la edita, aunque sea lead de su workspace" \
  "$(code -X PATCH "$API/api/collections/inbox_items/records/$IN2ID" -H "Authorization: $B" -H "$JS" -d '{"note":"secuestrada"}')" 404
chk ">>> el lead global sí (triar es su trabajo)" \
  "$(code -X PATCH "$API/api/collections/inbox_items/records/$IN2ID" -H "Authorization: $C" -H "$JS" -d '{"note":"triada"}')" 200

# El inbox se LEE como se edita: quien capturó, y quien tría. Capturar sigue
# abierto a todo el mundo — una nota que escribes y no vuelves a ver es una nota
# que se deja de escribir.
chk ">>> quien capturó lee su propia nota" \
  "$(curl -s "$API/api/collections/inbox_items/records/$IN2ID" -H "Authorization: $A" | j "['id']")" "$IN2ID"
chk ">>> pero no la de otro" \
  "$(code "$API/api/collections/inbox_items/records/$IN1ID" -H "Authorization: $A")" 404
# Tres notas: la de bob, la de alice, y la que alice intentó firmar como bob y
# quedó firmada como suya.
chk ">>> el lead global ve el inbox entero" \
  "$(curl -s "$API/api/collections/inbox_items/records" -H "Authorization: $C" | j "['totalItems']")" 3
chk "...y alice sólo lo suyo" \
  "$(curl -s "$API/api/collections/inbox_items/records" -H "Authorization: $A" | j "['totalItems']")" 2

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
# Las dos escalas conviven en la BURBUJA, y nunca mezcladas: la banda la deriva
# la evidencia de sus threads, la prioridad la decide quien orquesta.
BU_P=$(post bubbles "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Con prioridad\",\"impact\":\"high\",\"urgency\":\"high\"}" | j "['id']")
chk ">>> el board trae las dos escalas por separado" \
  "$(curl -s "$API/api/workspaces/$ALPHA/board" -H "Authorization: $A" | python3 -c 'import sys,json
d=json.load(sys.stdin)
b=next(b for b in d["bubbles"] if b["id"]=="'"$BU_P"'")
print("si" if b["priority"]=="P1" and b["heat"]["lifecycle"] in ("hot","dormant","rip") else b)')" si
chk ">>> ...y un thread ya no lleva prioridad: no es suya" \
  "$(echo "$BOARD" | python3 -c 'import sys,json
d=json.load(sys.stdin)
t=next(t for t in d["threads"] if t["id"]=="'"$T5"'")
print("priority" in t)')" False
chk "el board dice contra qué calibración clasificó" \
  "$(echo "$BOARD" | python3 -c 'import sys,json;print(int(json.load(sys.stdin)["tuning"]["cycle_hours"]))')" 168
chk "erin no ve el board" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/workspaces/$ALPHA/board" -H "Authorization: $ER")" 404

# Todos: las burbujas de cada workspace que alguien alcanza, en un solo board.
# Lo agrega el SERVIDOR — un reloj y una calibración — porque el calor es función
# del tiempo y dos filas medidas contra dos "ahora" no son comparables, que es
# justo lo único que esta pantalla existe para hacer.
echo
ALL_B=$(curl -s "$API/api/board" -H "Authorization: $B")
chk ">>> el board de TODOS junta los workspaces de quien mira" \
  "$(echo "$ALL_B" | python3 -c 'import sys,json
print(len(json.load(sys.stdin)["workspaces"]))')" 2
chk ">>> ...y cada fila dice de qué workspace es" \
  "$(echo "$ALL_B" | python3 -c 'import sys,json
d=json.load(sys.stdin)
print("si" if d["bubbles"] and all(b.get("workspace") for b in d["bubbles"]) else "no")')" si
chk ">>> ...con el slug, que es de lo que se hace la dirección" \
  "$(echo "$ALL_B" | python3 -c 'import sys,json
print("si" if all(w.get("slug") for w in json.load(sys.stdin)["workspaces"]) else "no")')" si
chk ">>> ...ordenado por banda a través de proyectos, no agrupado por proyecto" \
  "$(echo "$ALL_B" | python3 -c 'import sys,json
band={"hot":3,"dormant":2,"rip":1,"closed":0}
v=[band[b["heat"]["lifecycle"]] for b in json.load(sys.stdin)["bubbles"]]
print("si" if v==sorted(v,reverse=True) else v)')" si
chk ">>> alice sólo ve el suyo: el board de todos NO cruza la frontera" \
  "$(curl -s "$API/api/board" -H "Authorization: $A" | python3 -c 'import sys,json
d=json.load(sys.stdin)
print("si" if [w["slug"] for w in d["workspaces"]]==["alpha"] else d["workspaces"])')" si
chk ">>> carol (lead global) los ve todos sin ser miembro" \
  "$(curl -s "$API/api/board" -H "Authorization: $C" | python3 -c 'import sys,json
print(len(json.load(sys.stdin)["workspaces"]))')" 2
chk ">>> erin, que no es de ninguno, recibe un board vacío y no un error" \
  "$(curl -s "$API/api/board" -H "Authorization: $ER" | python3 -c 'import sys,json
d=json.load(sys.stdin)
print("si" if d["workspaces"]==[] and d["bubbles"]==[] else d)')" si
chk ">>> anónimo no ve el board de todos" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/board")" 401

# Cerrar una burbuja es una DECISIÓN con fecha, no un borrado: baja a la banda
# de cerradas, dice cómo terminó, y se puede reabrir. Y quién está a cargo es una
# LISTA, porque el trabajo compartido es la norma y la banda 🪦 solo pregunta si
# hay alguien.
echo
BU_X=$(post bubbles "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Se cierra\"}" | j "['id']")
onbubble(){ curl -s -o /dev/null -w '%{http_code}' -X PATCH "$API/api/collections/bubbles/records/$BU_X" -H "Authorization: $1" -H "$JS" -d "$2"; }
bubbleon(){ curl -s "$API/api/workspaces/$ALPHA/board" -H "Authorization: $A" | python3 -c "import sys,json
d=json.load(sys.stdin); b=next(x for x in d['bubbles'] if x['id']=='$BU_X'); print($1)"; }

chk ">>> una burbuja recién creada no está cerrada" "$(bubbleon "b['closed']")" False
chk ">>> cerrada, el board la pone en la banda closed" \
  "$(onbubble "$A" '{"closed_at":"2026-01-02 03:04:05.000Z","closure":"se resolvió por otro lado"}' >/dev/null; bubbleon "b['heat']['lifecycle']")" closed
chk ">>> ...y carga la frase de quien la cerró" "$(bubbleon "b.get('closure','')")" "se resolvió por otro lado"
chk ">>> reabrir la devuelve a una banda viva" "$(onbubble "$A" '{"closed_at":"","closure":""}')" 200
chk "...y el board ya no la da por cerrada" "$(bubbleon "b['closed']")" False

# El cuerpo va en una variable, como el resto del archivo: escrito en línea
# dentro de "$( ... )" el shell se come el escape de las comillas y el JSON llega
# roto, que es un 400 genérico y una tarde perdida.
DOS="{\"owners\":[\"$BID\",\"$AID\"]}"
chk ">>> a cargo es una LISTA: dos personas caben en la misma burbuja" "$(onbubble "$B" "$DOS")" 200
chk "...y el board las devuelve las dos" "$(bubbleon "len(b.get('owners',[]))")" 2
chk ">>> vaciar la lista es una respuesta válida: nadie a cargo" "$(onbubble "$B" '{"owners":[]}')" 200
chk ">>> el board dice cuándo pasó algo en cada thread, para poder decir la edad" \
  "$(curl -s "$API/api/workspaces/$ALPHA/board" -H "Authorization: $A" | python3 -c 'import sys,json
d=json.load(sys.stdin); print("si" if all(t.get("at") for t in d["threads"]) else "no")')" si
# Presencia: un latido, y nada más. No es evidencia, no calienta nada, y no se
# escribe desde un cliente — el reloj es del servidor.
echo
chk ">>> latir crea la fila de quien late" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/presence" -H "Authorization: $A")" 200
chk "...y latir otra vez la actualiza en vez de duplicarla" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/presence" -H "Authorization: $A" >/dev/null
     curl -s "$API/api/collections/presence/records" -H "Authorization: $A" | j "['totalItems']")" 1
chk ">>> cualquiera que trabaje aquí ve quién está" \
  "$(curl -s "$API/api/collections/presence/records" -H "Authorization: $ER" | j "['totalItems']")" 1
chk ">>> anónimo no late" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/presence")" 401
chk ">>> anónimo tampoco ve quién está" \
  "$(curl -s "$API/api/collections/presence/records" | j "['totalItems']")" 0
PRBODY="{\"user\":\"$AID\",\"at\":\"2030-01-01 00:00:00.000Z\"}"
PRCODE=$(pcode presence "$A" "$PRBODY")
chk ">>> nadie escribe presencia desde un cliente: el reloj es del servidor" "$PRCODE" 403
chk ">>> latir NO es evidencia: no deja evento" \
  "$(curl -s "$API/api/collections/events/records?filter=$(python3 -c "import urllib.parse;print(urllib.parse.quote(\"kind='presence'\"))")" \
     -H "Authorization: $SU" | j "['totalItems']")" 0

chk ">>> una persona nueva es nombrable: su correo es visible para sus colegas" \
  "$(curl -s "$API/api/collections/users/records/$AID" -H "Authorization: $B" | j "['email']")" alice@bubble.test
chk "...y el roster del workspace se lee expandido, para poder elegir a quién" \
  "$(curl -s "$API/api/collections/memberships/records?expand=user&filter=$(python3 -c "import urllib.parse;print(urllib.parse.quote(\"workspace='$ALPHA'\"))")" \
     -H "Authorization: $A" | python3 -c 'import sys,json
d=json.load(sys.stdin); print("si" if d["items"] and d["items"][0].get("expand",{}).get("user",{}).get("display_name") else "no")')" si

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

# Qué está corriendo aquí. Sin sesión y sin tocar la base: es la comprobación de
# salud del contenedor y lo que mira el actualizador antes y después de cambiar
# la imagen.
# Actualizarse sola. Lo que se prueba aquí son las FRONTERAS —quién puede
# pedirlo y qué contesta cuando no se puede— no la descarga: bajar un release de
# verdad haría de esta suite una que falla cuando GitHub tiene un mal día.
#
# El arranque vigilante se prueba en Go (`internal/boot`), que es donde vive la
# tabla de qué hacer cuando el hijo se muere.
chk ">>> actualizar es del lead del departamento, no de un miembro" \
  "$(code -X POST "$API/api/update" -H "Authorization: $A" -H "$JS" -d '{}')" 403
chk ">>> ...ni de un anónimo" \
  "$(code -X POST "$API/api/update" -H "$JS" -d '{}')" 403
# Esta suite arranca con `serve`, no con `boot`: no hay quién deshaga una
# actualización, y el servidor tiene que DECIRLO en vez de aceptarla.
chk ">>> sin arranque vigilante se rechaza, y se explica" \
  "$(code -X POST "$API/api/update" -H "Authorization: $C" -H "$JS" -d '{}')" 409
chk ">>> ...y /api/version lo dice, para que la UI no ofrezca un botón inerte" \
  "$(curl -s "$API/api/version" | j "['boot']")" False

chk ">>> /api/version contesta sin sesión" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/version")" 200
chk ">>> ...y dice qué versión es" \
  "$(curl -s "$API/api/version" | python3 -c 'import sys,json;print(bool(json.load(sys.stdin).get("version")))')" True

chk ">>> tools/list expone la superficie" \
  "$(mcp "$A" "tools/list" "{}" | python3 -c 'import sys,json
n=sorted(t["name"] for t in json.load(sys.stdin)["result"]["tools"])
print(",".join(n))')" \
  "agents_md,artifact_append,artifact_create,artifact_delete,artifact_handoff,artifact_list,artifact_pin,artifact_read,artifact_wait,bubble_close,bubble_create,bubble_reopen,bubble_update,change_list,checkbox_toggle,comment_create,comment_list,document_delete,document_edit,document_list,document_read,document_replace,document_search,evidence_add,inbox_capture,inbox_delete,inbox_list,inbox_read,inbox_triage,inbox_update,inventory_group_create,inventory_group_delete,inventory_group_update,inventory_item_create,inventory_item_delete,inventory_item_update,inventory_list,objective_create,objective_list,objective_update,people_list,pin_add,pin_list,pin_remove,project_board,project_create,project_list,repo_link,repo_list,repo_unlink,sequence_next,sequence_set,space_note_arrange,space_note_create,space_note_delete,space_note_list,space_note_update,team_list,thread_complete,thread_create,thread_get,thread_history,thread_relate,thread_unrelate,thread_update"
chk ">>> cada tool dice si sólo lee (anotaciones MCP)" \
  "$(mcp "$A" "tools/list" "{}" | python3 -c 'import sys,json
t={x["name"]:x.get("annotations",{}) for x in json.load(sys.stdin)["result"]["tools"]}
print(t["document_read"].get("readOnlyHint"), t["document_delete"].get("destructiveHint"), t["document_edit"].get("destructiveHint"), bool(t["thread_create"].get("title")))')" "True True False True"
# La plantilla que una persona copia para configurar su asistente; lleva dentro
# la sección breve que el agente se instala. Markdown servido, no cadenas dentro
# del código.
chk ">>> el prompt de conexión sale SIN rellenar: los huecos los pone el cliente" \
  "$(curl -s "$API/api/connect" | grep -c '{{token}}')" 1
chk ">>> ...y trae la sección que un agente se instala, con el hueco del proyecto" \
  "$(curl -s "$API/api/connect" | grep -c '<SLUG>')" 2
chk ">>> ...y la sección obliga a leer agents_md" \
  "$(curl -s "$API/api/connect" | grep -c 'llama a `agents_md` y síguela')" 1
chk ">>> house_rules ya no existe: era una segunda copia" \
  "$(curl -s "$API/api/house-rules" | grep -c '<SLUG>')" 0

# ---- AGENTS.md del departamento: lo escribe el lead, lo lee cada agente ----
chk ">>> sin versiones, agents_md lo dice en vez de fallar" \
  "$(mcptext "$A" agents_md '{}' | head -1)" "AGENTS.md (operators) · todavía no hay ninguna versión"
chk ">>> un member NO crea una versión" \
  "$(pcode agents_md "$A" "{\"role\":\"operators\",\"content\":\"x\",\"author\":\"$AID\"}")" 400
chk ">>> el lead global NO firma una versión a nombre de otro" \
  "$(pcode agents_md "$C" "{\"role\":\"operators\",\"content\":\"x\",\"author\":\"$AID\"}")" 400
chk ">>> el lead global sí crea la suya" \
  "$(pcode agents_md "$C" "{\"role\":\"operators\",\"content\":\"# WoW v1\",\"author\":\"$CID\"}")" 200
post agents_md "$C" "{\"role\":\"operators\",\"content\":\"# WoW v2\",\"author\":\"$CID\"}" >/dev/null
post agents_md "$C" "{\"role\":\"planner\",\"content\":\"# Plan v1\",\"author\":\"$CID\"}" >/dev/null
# La forma de trabajar del departamento va AL FINAL, después de la guía: lo
# último que lee el agente es lo que manda.
lastdept(){ sed -n '/^# Forma de trabajar del departamento/,$p' | sed -n 3p; }
chk ">>> un member recibe la VIGENTE de operadores" \
  "$(mcptext "$A" agents_md '{}' | lastdept)" "# WoW v2"
chk "...con autor en la cabecera" \
  "$(mcptext "$A" agents_md '{}' | head -1 | grep -c 'por Carol')" 1
chk ">>> el lead recibe la de planner" \
  "$(mcptext "$C" agents_md '{}' | lastdept)" "# Plan v1"
chk "...y puede leer la de operadores" \
  "$(mcptext "$C" agents_md '{"role":"operators"}' | lastdept)" "# WoW v2"
chk ">>> agents_md trae la guía de Bubble" \
  "$(mcptext "$A" agents_md '{}' | sed -n 3p)" "# Bubble Work — guía para agentes"
chk ">>> ...las entidades, con wiki, artefactos y pines" \
  "$(mcptext "$A" agents_md '{}' | grep -cE '^- \*\*(Proyecto|Módulo|Hilo|Artefactos temporales|Pines)\*\*')" 5
chk ">>> ...y la tabla de tools, generada del registro" \
  "$(mcptext "$A" agents_md '{}' | grep -cE '^\| `[a-z_]+` \|')" 65
chk ">>> ...por categoría" \
  "$(mcptext "$A" agents_md '{}' | grep -c '^### Módulos (burbujas)$')" 1
chk ">>> ...y el protocolo de interrogatorio NO: llega al crear el artefacto" \
  "$(mcptext "$A" agents_md '{}' | grep -c '^# Interrogatorio a través de Bubble')" 0
AGV=$(list agents_md "$C" | j "['items'][0]['id']")
chk ">>> una versión no se edita" \
  "$(code -X PATCH "$API/api/collections/agents_md/records/$AGV" -H "Authorization: $C" -H "$JS" -d '{"content":"otra"}')" 403
chk ">>> ni se borra" \
  "$(code -X DELETE "$API/api/collections/agents_md/records/$AGV" -H "Authorization: $C")" 403

chk ">>> la guía es prompt Y tool (no todo cliente lista prompts)" \
  "$(mcp "$A" "prompts/list" "{}" | python3 -c 'import sys,json
print(",".join(p["name"] for p in json.load(sys.stdin)["result"]["prompts"]))')" bubble-work
chk "/api/guide sirve la misma guía que agents_md" \
  "$(curl -s "$API/api/guide" | head -1)" "# Bubble Work — guía para agentes"
chk "sin token, /mcp no responde" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/mcp" -H "$JS" -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}')" 401

chk ">>> project_list respeta la frontera: erin no ve ninguno" \
  "$(mcptext "$ER" project_list '{}' | python3 -c 'import sys,json
try: print(len(json.loads(sys.stdin.read()) or []))
except Exception: print(0)')" 0

# ---- el bucle completo de un agente, por slug ----
echo
NT=$(mcptext "$A" thread_create '{"project":"alpha","name":"Trabajo del agente"}')
NTID=$(echo "$NT" | j "['id']")
chk ">>> un agente crea un thread por SLUG, sin conocer ids" "$([ -n "$NTID" ] && echo si || echo no)" si
chk "...y el server le puso número y ruta" "$(echo "$NT" | j "['doc_path']")" "threads/$(echo "$NT" | j "['seq']")-trabajo-del-agente.md"

# `read` y `edit` contestan MARKDOWN, con una cabecera de dos líneas delante. El
# hash se saca de ahí — que es exactamente lo que hará un agente.
hashof(){ echo "$1" | sed -nE 's/.*· hash ([a-f0-9]+).*/\1/p' | head -1; }

RD=$(mcptext "$A" document_read "{\"thread\":\"$NTID\"}")
H=$(hashof "$RD")
chk "read devuelve el hash que la escritura necesita" "$([ -n "$H" ] && echo si || echo no)" si
chk ">>> ...y dice que hay que devolverlo al escribir" \
  "$(echo "$RD" | grep -c 'base')" 1
chk ">>> un documento vacío se dice, no se calla" \
  "$(echo "$RD" | grep -c 'vacío')" 1

W1=$(mcptext "$A" document_replace "{\"thread\":\"$NTID\",\"base\":\"$H\",\"content\":\"# Trabajo\\n\\n- [ ] investigar el rate limit\\n\"}")
chk ">>> escribe el documento" "$(echo "$W1" | grep -c 'investigar el rate limit')" 1
chk ">>> ...y vuelve como markdown de verdad, sin \\n escapados" \
  "$(echo "$W1" | grep -c '^# Trabajo$')" 1
H2=$(hashof "$W1")
chk ">>> ...con el hash NUEVO, para encadenar otro cambio sin releer" \
  "$([ -n "$H2" ] && [ "$H2" != "$H" ] && echo si || echo no)" si
chk ">>> escribir con el base viejo se rechaza también por MCP" \
  "$(mcptext "$A" document_replace "{\"thread\":\"$NTID\",\"base\":\"$H\",\"content\":\"pisado\"}" | grep -c "changed since you read it")" 1
chk ">>> document_edit sin edits se rechaza en vez de escribir nada" \
  "$(mcptext "$A" document_edit "{\"thread\":\"$NTID\",\"base\":\"$H2\",\"edits\":[]}" | grep -c 'está vacío')" 1
W2=$(mcptext "$A" checkbox_toggle "{\"thread\":\"$NTID\",\"base\":\"$H2\",\"index\":0,\"done\":true}")
chk ">>> marca la casilla y devuelve el conteo" "$(echo "$W2" | grep -c '1 hechas')" 1
# El html existe para la aplicación web, que lo pide por la API. Por esta puerta
# duplicaba el tamaño sin que ningún agente lo leyera.
chk ">>> y el html NO viaja por MCP" "$(echo "$W2" | grep -c '<h1>')" 0

# Leer un TROZO. Un documento de 38 KB son diez mil tokens cada vez que alguien
# quiere corregir una línea; `search` dice en qué línea está y `read` la trae.
#
# En una página aparte, no en el documento del thread: una prueba que pisa lo que
# escribió la anterior deja a las siguientes comprobando otra cosa.
LONG=$(python3 -c "print('\\\\n'.join('linea %d' % i for i in range(1, 61)))")
ARG="{\"project\":\"alpha\",\"path\":\"docs/largo.md\",\"base\":\"$E0\",\"content\":\"$LONG\"}"
W3=$(mcptext "$A" document_replace "$ARG")
H3=$(hashof "$W3")
RD2=$(mcptext "$A" document_read '{"project":"alpha","path":"docs/largo.md","from":30,"lines":3}')
chk ">>> read trae sólo el trozo que se pidió" "$(echo "$RD2" | grep -c '^linea 3[012]$')" 3
chk ">>> ...y NO lo que está fuera de él" "$(echo "$RD2" | grep -c '^linea 29$')" 0
chk ">>> ...diciendo qué trozo es y de cuántas líneas" \
  "$(echo "$RD2" | grep -c 'líneas 30–32 de 60')" 1
chk ">>> ...y cómo pedir el resto" "$(echo "$RD2" | grep -c 'from: 33')" 1
chk ">>> el hash es el del documento ENTERO, o una escritura parcial sería falsa" \
  "$(hashof "$RD2")" "$H3"
chk ">>> pedir más allá del final devuelve lo que hay, no un error" \
  "$(mcptext "$A" document_read '{"project":"alpha","path":"docs/largo.md","from":900,"lines":5}' | grep -c '^linea 60$')" 1
chk ">>> search dice en qué línea, para poder leer sólo esa parte" \
  "$(mcptext "$A" document_search '{"project":"alpha","query":"linea 42"}' | j "[0]['line']")" 42
chk ">>> ...y con around trae el contexto para citarlo en un edit" \
  "$(mcptext "$A" document_search '{"project":"alpha","query":"linea 42","around":1}' | j "[0]['around']" | grep -c 'linea 41')" 1
chk ">>> ...y sin pedirlo no viene" \
  "$(mcptext "$A" document_search '{"project":"alpha","query":"linea 42"}' | grep -c 'around')" 0

chk ">>> search encuentra lo que acaba de escribir" \
  "$(mcptext "$A" document_search '{"project":"alpha","query":"rate limit"}' | python3 -c 'import sys,json
h=json.loads(sys.stdin.read()) or []
print("si" if any("trabajo-del-agente" in x["path"] for x in h) else "no")')" si
chk ">>> link: evidencia externa" \
  "$(mcptext "$A" evidence_add "{\"thread\":\"$NTID\",\"url\":\"https://example.com/pr/9\",\"title\":\"PR\"}" | j "['url']")" "https://example.com/pr/9"

chk ">>> y la burbuja se calienta por eso: el thread sale hot" \
  "$(mcptext "$A" project_board '{"project":"alpha"}' | python3 -c 'import sys,json
d=json.loads(sys.stdin.read())
print(next(t["heat"]["lifecycle"] for t in d["threads"] if t["id"]=="'"$NTID"'"))')" hot
chk "...y la prioridad vive en su burbuja, no en él" \
  "$(mcptext "$A" project_board '{"project":"alpha"}' | python3 -c 'import sys,json
d=json.loads(sys.stdin.read())
print(next(b["priority"] for b in d["bubbles"] if b["id"]=="'"$BU_P"'"))')" P1
chk ">>> completar es un cambio de estado, no un examen" \
  "$(mcptext "$A" thread_complete "{\"thread\":\"$NTID\"}" | j "['id']")" "$NTID"
chk "...y el board lo refleja" \
  "$(mcptext "$A" project_board '{"project":"alpha"}' | python3 -c 'import sys,json
d=json.loads(sys.stdin.read())
print(next(t["heat"]["lifecycle"] for t in d["threads"] if t["id"]=="'"$NTID"'"))')" closed

chk ">>> erin no alcanza el thread por MCP" \
  "$(mcptext "$ER" document_read "{\"thread\":\"$NTID\"}" | grep -c "not found")" 1

# La otra mitad de la superficie: un agente que sólo escribe el documento
# trabaja a ciegas — no puede decir para qué es el trabajo, cuándo vence, quién
# responde por la burbuja que lo contiene, ni leer lo que ya se hizo.
#
# Cada cuerpo va en una VARIABLE. Escrito en línea dentro de "$( ... )" el shell
# se come el escape de las comillas y el JSON llega roto; el servidor contesta
# "malformed payload" y la prueba parece pasar contra la nada.
echo
# Fundar por MCP tiene que hacer TODO lo que fundar significa: repositorio,
# membresía de quien funda, y el flujo de trabajo. Los ganchos que hacen eso en
# la web cuelgan de la PETICIÓN, y esta puerta no pasa por una.
ARG="{\"name\":\"Laboratorio\"}"
NWS=$(mcptext "$B" project_create "$ARG" | j "['slug']")
chk ">>> un agente funda un workspace" "$NWS" laboratorio
chk ">>> ...y queda como su lead" \
  "$(curl -s "$API/api/collections/memberships/records?filter=$(python3 -c "import urllib.parse;print(urllib.parse.quote(\"workspace.slug='laboratorio'\"))")" \
     -H "Authorization: $B" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["items"][0]["role"] if d["items"] else "sin membresía")')" lead
chk ">>> ...con su flujo de trabajo, o un thread no tendría dónde estar" \
  "$(curl -s "$API/api/collections/states/records?filter=$(python3 -c "import urllib.parse;print(urllib.parse.quote(\"workspace.slug='laboratorio'\"))")" \
     -H "Authorization: $B" | j "['totalItems']")" 3
chk ">>> ...y alice no lo ve: fundar no lo hace de todos" \
  "$(mcptext "$A" document_list "{\"project\":\"laboratorio\"}" | grep -c "not found")" 1

ARG="{\"project\":\"alpha\",\"name\":\"Portal\",\"outcome\":\"la gente entra sin pedir ayuda\"}"
NBUB=$(mcptext "$A" bubble_create "$ARG" | j "['id']")
chk ">>> un agente crea una burbuja con su outcome" "$([ -n "$NBUB" ] && echo si || echo no)" si

ARG="{\"bubble\":\"$NBUB\",\"owners\":[\"$AID\"]}"
chk ">>> ...y dice quién responde por ella" \
  "$(mcptext "$A" bubble_update "$ARG" | python3 -c 'import sys,json;print(len(json.load(sys.stdin)["owners"]))')" 1

ARG="{\"bubble\":\"$NBUB\",\"closure\":\"\"}"
chk ">>> cerrar sin decir cómo terminó se rechaza" \
  "$(mcptext "$A" bubble_close "$ARG" | grep -c 'di cómo terminó')" 1
ARG="{\"bubble\":\"$NBUB\",\"closure\":\"se resolvió en soporte\"}"
chk ">>> cerrar es una decisión con frase, no un borrado" \
  "$(mcptext "$A" bubble_close "$ARG" | j "['closed']")" True
ARG="{\"bubble\":\"$NBUB\"}"
chk ">>> ...y se reabre" "$(mcptext "$A" bubble_reopen "$ARG" | j "['closed']")" False

ARG="{\"thread\":\"$NTID\",\"bubble\":\"$NBUB\"}"
chk ">>> el thread se mueve a esa burbuja" "$(mcptext "$A" thread_update "$ARG" | j "['bubble']")" "$NBUB"
# La prioridad es de la BURBUJA: la decide quien orquesta, y el operador ya tiene
# su propia forma de ordenarse el día.
ARG="{\"bubble\":\"$NBUB\",\"impact\":\"high\",\"urgency\":\"mid\"}"
chk ">>> la prioridad se le pone a la burbuja" \
  "$(mcptext "$A" bubble_update "$ARG" | j "['impact']")" high
chk ">>> ...y la deriva el servidor de impacto x urgencia" \
  "$(curl -s "$API/api/collections/bubble_priority/records/$NBUB" -H "Authorization: $A" | j "['priority']")" P2
# La fecha también es de la burbuja: cuándo tiene que estar ese cuerpo de
# trabajo. Cómo lo reparte entre sus threads es de quien ejecuta.
ARG="{\"bubble\":\"$NBUB\",\"due\":\"2026-12-31\"}"
chk ">>> la fecha se le pone a la burbuja" \
  "$(mcptext "$A" bubble_update "$ARG" | j "['due_date'][:10]")" 2026-12-31
chk ">>> ...y el calendario la lee de ahí" \
  "$(curl -s "$API/api/collections/bubbles/records/$NBUB" -H "Authorization: $A" | j "['due_date'][:10]")" 2026-12-31
ARG="{\"bubble\":\"$NBUB\",\"due\":\"31/12/2026\"}"
chk ">>> una fecha mal formada se rechaza en vez de guardarse rara" \
  "$(mcptext "$A" bubble_update "$ARG" | grep -c "a due date is a day")" 1
# La fecha de la BURBUJA es el compromiso; la de la tarea es cómo se reparte, y
# son independientes a propósito. Con cinco tareas dentro de una burbuja que
# vence el 30, las cinco parecen vencer el 30 y la que bloquea a las otras el
# martes no se distingue.
chk ">>> una tarea vuelve a tener fecha propia" \
  "$(curl -s -X PATCH "$API/api/collections/threads/records/$NTID" -H "Authorization: $A" \
     -H 'Content-Type: application/json' -d '{"due_date":"2026-12-24 00:00:00.000Z"}' \
     | python3 -c 'import sys,json;print(json.load(sys.stdin)["due_date"][:10])')" 2026-12-24
chk ">>> ...y NO toca la de su burbuja: una es el compromiso, la otra el reparto" \
  "$(curl -s "$API/api/collections/bubbles/records/$NBUB" -H "Authorization: $A" \
     | python3 -c 'import sys,json;print(json.load(sys.stdin)["due_date"][:10])')" 2026-12-31

chk ">>> el board lleva el plazo propio del hilo al calendario" \
  "$(curl -s "$API/api/workspaces/$ALPHA/board" -H "Authorization: $A" | python3 -c "import sys,json;print(next(t.get('due_date','') for t in json.load(sys.stdin)['threads'] if t['id']=='$NTID'))")" 2026-12-24
chk ">>> el board agregado conserva también ese plazo" \
  "$(curl -s "$API/api/board" -H "Authorization: $A" | python3 -c "import sys,json;print(next(t.get('due_date','') for t in json.load(sys.stdin)['threads'] if t['id']=='$NTID'))")" 2026-12-24

# El inbox es de las PERSONAS: se captura y se tría en la aplicación. Un agente
# que trabaja desde el inbox trabaja fuera de la pieza que alguien asumió, así que
# la puerta no existe — ni para escribir ni para leer.
chk ">>> un agente NO captura: la tool no existe" \
  "$(mcptext "$B" capture "{\"note\":\"el portal tarda en cargar\"}" | python3 -c 'import sys,json
d=json.load(sys.stdin); print("error" if "error" in d or not d.get("result",{}).get("content") else "creada")')" error

ARG="{\"name\":\"Soporte sin fricción\",\"outcome\":\"nadie escribe dos veces\"}"
chk ">>> el lead global crea un objetivo" \
  "$(mcptext "$C" objective_create "$ARG" | python3 -c 'import sys,json;print("si" if json.load(sys.stdin).get("id") else "no")')" si
ARG="{\"name\":\"Mío\"}"
chk ">>> y un lead de workspace NO: la capa estratégica es del departamento" \
  "$(mcptext "$A" objective_create "$ARG" | grep -c "not found")" 1

chk ">>> el plan: los objetivos son del lead global" \
  "$(mcptext "$C" objective_list "{}" | python3 -c 'import sys,json;print(len(json.load(sys.stdin)["objectives"]) >= 1)')" True
chk ">>> ...quien no lo es ve el plan sin objetivos" \
  "$(mcptext "$A" objective_list "{}" | python3 -c 'import sys,json;print(len(json.load(sys.stdin)["objectives"]))')" 0
chk ">>> ...y el inbox NO cruza el MCP, ni el suyo" \
  "$(mcptext "$B" objective_list "{}" | python3 -c 'import sys,json;print("inbox" in json.load(sys.stdin))')" False

# Comentar es PULSO: mantiene al thread fuera de la tumba y NO lo calienta. Es
# justo lo que permite que un agente diga "busqué y no había nada" sin fingir
# que produjo evidencia.
ARG="{\"thread\":\"$NTID\",\"body\":\"Revisé el log y no encontré el error\"}"
chk ">>> un agente comenta" \
  "$(mcptext "$A" comment_create "$ARG" | python3 -c 'import sys,json;print("si" if json.load(sys.stdin).get("id") else "no")')" si
ARG="{\"thread\":\"$NTID\"}"
chk ">>> ...y lee el hilo" \
  "$(mcptext "$A" comment_list "$ARG" | python3 -c 'import sys,json;print(len(json.load(sys.stdin)) >= 1)')" True
chk ">>> comentar NO calienta: queda como pulso" \
  "$(mcptext "$A" project_board "{\"project\":\"alpha\"}" | python3 -c 'import sys,json
t=next(x for x in json.load(sys.stdin)["threads"] if x["id"]=="'"$NTID"'")
print("si" if t["pulse"] else t)')" si
chk ">>> el stream filtra por la regla: erin no recibe lo que no puede leer" \
  "$(curl -s "$API/api/collections/comments/records?perPage=1" -H "Authorization: $ER" | j "['totalItems']")" 0
chk ">>> erin no comenta en un thread que no alcanza" \
  "$(mcptext "$ER" comment_create "{\"thread\":\"$NTID\",\"body\":\"hola\"}" | grep -c "not found")" 1

ARG="{\"thread\":\"$NTID\"}"
chk ">>> timeline: qué le pasó al thread y cuándo" \
  "$(mcptext "$A" thread_history "$ARG" | python3 -c 'import sys,json
ks=[m["kind"] for m in json.load(sys.stdin)]
print("si" if "thread-created" in ks and "document-changed" in ks else ks)')" si
chk ">>> erin no ve el timeline de un thread que no alcanza" \
  "$(mcptext "$ER" thread_history "$ARG" | grep -c "not found")" 1

ARG="{\"project\":\"alpha\",\"path\":\"docs/agente.md\",\"base\":\"$E0\",\"content\":\"# Del agente\\n\"}"
chk ">>> un agente escribe una página de wiki" \
  "$(mcptext "$A" document_replace "$ARG" | grep -c '^docs/agente.md · hash ')" 1
ARG="{\"project\":\"alpha\",\"path\":\"docs/agente.md\"}"
chk "...y la borra" "$(mcptext "$A" document_delete "$ARG" | j "['deleted']")" True
ARG="{\"project\":\"alpha\",\"path\":\"threads/1-renombrado.md\"}"
chk ">>> pero el documento propio de un thread no se borra por esa puerta" \
  "$(mcptext "$A" document_delete "$ARG" | grep -c "goes when the thread does")" 1

# ------------------------------- la IA: ideas, Mi espacio, pines, ficha… ----
# Lo que un agente puede hacer además de escribir documentos. Las mismas reglas
# que la aplicación, comprobadas aquí, porque `app.Save` no las conoce.
echo
chk ">>> people_list dice quién es quién, y cuál soy, sin correos" \
  "$(mcptext "$A" people_list '{}' | python3 -c 'import sys,json
d=json.load(sys.stdin); me=[p for p in d if p.get("me")]
print(len(me), me[0]["id"]=="'"$AID"'", any("email" in p for p in d))')" "1 True False"

EV_IDEA=$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")
IDEA=$(mcptext "$B" inbox_capture '{"note":"Probar un modo oscuro en el portal","body":"lo pidió un cliente"}')
IDEAID=$(echo "$IDEA" | j "['id']")
chk ">>> un agente captura una IDEA en el inbox de su persona" "$(echo "$IDEA" | j "['source']")" app
chk "...firmada por su persona" \
  "$(curl -s "$API/api/collections/inbox_items/records/$IDEAID" -H "Authorization: $B" | j "['captured_by']")" "$BID"
chk "...y una idea NO es trabajo: ningún evento" \
  "$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")" "$EV_IDEA"
WAIDEA=$(curl -s -X POST "$API/api/collections/inbox_items/records" -H "Authorization: $SU" -H "$JS" \
  -d "{\"note\":\"¿Hay demo el jueves?\",\"body\":\"¿Hay demo el jueves?\\nLo pregunta el cliente\",\"captured_by\":\"$BID\",\"team\":\"$TI\",\"source\":\"whatsapp\",\"source_ref\":\"t-ai-1\",\"source_from\":\"Ana Ventas\"}" | j "['id']")
chk ">>> lo que llega por WhatsApp sale como idea, con quién lo mandó" \
  "$(mcptext "$B" inbox_list '{"source":"whatsapp"}' | python3 -c 'import sys,json
d=[x for x in json.load(sys.stdin) if x["id"]=="'"$WAIDEA"'"]; print(d[0]["from"], d[0]["source"], d[0]["decided"])')" "Ana Ventas whatsapp False"
chk "...y su mensaje entero se lee" \
  "$(mcptext "$B" inbox_read "{\"idea\":\"$WAIDEA\"}" | j "['body']" | grep -c 'Lo pregunta el cliente')" 1
chk ">>> alice no ve las ideas de bob" \
  "$(mcptext "$A" inbox_read "{\"idea\":\"$IDEAID\"}" | grep -c 'not found')" 1
chk "...ni en su lista" \
  "$(mcptext "$A" inbox_list '{}' | python3 -c 'import sys,json;print(any(x["id"]=="'"$IDEAID"'" for x in json.load(sys.stdin)))')" False
chk ">>> el lead global las ve todas" \
  "$(mcptext "$C" inbox_list '{}' | python3 -c 'import sys,json;print(any(x["id"]=="'"$IDEAID"'" for x in json.load(sys.stdin)))')" True
chk ">>> la tool dice que una idea no es trabajo" \
  "$(mcp "$A" "tools/list" "{}" | python3 -c 'import sys,json
print([t for t in json.load(sys.stdin)["result"]["tools"] if t["name"]=="inbox_list"][0]["description"].count("no trabajo"))')" 1
ARG="{\"idea\":\"$IDEAID\",\"note\":\"Modo oscuro en el portal\"}"
chk ">>> corregir una idea" \
  "$(mcptext "$B" inbox_update "$ARG" | j "['note']")" "Modo oscuro en el portal"
ARG="{\"idea\":\"$IDEAID\",\"thread\":\"zzzzzzzzzzzzzzz\"}"
chk ">>> anotar a dónde fue exige un trabajo que existe" \
  "$(mcptext "$B" inbox_triage "$ARG" | grep -c 'not found')" 1
ARG="{\"idea\":\"$IDEAID\",\"thread\":\"$NTID\"}"
chk "...y entonces queda decidida, apuntando al hilo" \
  "$(mcptext "$B" inbox_triage "$ARG" | j "['decided']")" True
chk "...y deja de salir entre las que nadie ha decidido" \
  "$(mcptext "$B" inbox_list '{}' | python3 -c 'import sys,json;print(any(x["id"]=="'"$IDEAID"'" for x in json.load(sys.stdin)))')" False
chk ">>> descartar una idea" "$(mcptext "$B" inbox_delete "{\"idea\":\"$WAIDEA\"}" | j "['deleted']")" True

N1=$(mcptext "$A" space_note_create '{"title":"Ideas sueltas","color":"menta"}')
N1ID=$(echo "$N1" | j "['id']")
chk ">>> un agente pega una nota en Mi espacio, con un color por nombre" "$(echo "$N1" | j "['color']")" "#bcebd9"
N2=$(mcptext "$A" space_note_create '{"title":"Otra"}')
N2ID=$(echo "$N2" | j "['id']")
chk "...sin posición, queda a la derecha de las que hay" \
  "$(python3 -c "print($(echo "$N2" | j "['x']") > $(echo "$N1" | j "['x']"))")" True
ARG="{\"notes\":[{\"note\":\"$N1ID\",\"x\":0,\"y\":0},{\"note\":\"$N2ID\",\"x\":0,\"y\":300,\"w\":5000}]}"
chk ">>> acomodar varias de una vez" \
  "$(mcptext "$A" space_note_arrange "$ARG" | python3 -c 'import sys,json
d=json.load(sys.stdin); print(len(d), d[1]["y"], d[1]["w"])')" "2 300 1200"
ARG="{\"notes\":[{\"note\":\"$N1ID\",\"x\":10,\"y\":10},{\"note\":\"zzzzzzzzzzzzzzz\",\"x\":0,\"y\":0}]}"
chk ">>> ...todo o nada" \
  "$(mcptext "$A" space_note_arrange "$ARG" | grep -c 'not found')" 1
chk "...y la primera no se movió" \
  "$(mcptext "$A" space_note_list '{}' | python3 -c 'import sys,json;print([n["x"] for n in json.load(sys.stdin) if n["id"]=="'"$N1ID"'"][0])')" 0
ARG="{\"note\":\"$N1ID\",\"color\":\"verde chillón\"}"
chk ">>> un color que no es de la paleta se rechaza" \
  "$(mcptext "$A" space_note_update "$ARG" | grep -c 'paleta')" 1
ARG="{\"note\":\"$N1ID\",\"title\":\"mía\"}"
chk ">>> nadie más toca Mi espacio, ni el lead global" \
  "$(mcptext "$C" space_note_update "$ARG" | grep -c 'not found')" 1
chk "...ni lo ve en su lista" \
  "$(mcptext "$C" space_note_list '{}' | python3 -c 'import sys,json;print(any(n["id"]=="'"$N1ID"'" for n in json.load(sys.stdin)))')" False
chk ">>> borrar una nota" "$(mcptext "$A" space_note_delete "{\"note\":\"$N2ID\"}" | j "['deleted']")" True

pins_of(){ python3 -c 'import sys,json;print(sum(1 for p in json.load(sys.stdin) if p["thread"]=="'"$NTID"'"))'; }
chk ">>> fijar un hilo" "$(mcptext "$A" pin_add "{\"thread\":\"$NTID\"}" | pins_of)" 1
chk "...dos veces no lo duplica" "$(mcptext "$A" pin_add "{\"thread\":\"$NTID\"}" | pins_of)" 1
chk "...y la ficha lo dice" "$(mcptext "$A" thread_get "{\"thread\":\"$NTID\"}" | j "['pinned']")" True
chk ">>> erin no fija lo que no alcanza" "$(mcptext "$ER" pin_add "{\"thread\":\"$NTID\"}" | grep -c 'not found')" 1
chk ">>> soltarlo" "$(mcptext "$A" pin_remove "{\"thread\":\"$NTID\"}" | pins_of)" 0
PREFROW=$(curl -s "$API/api/collections/prefs/records" -H "Authorization: $A" | j "['items'][0]['id']")
curl -s -o /dev/null -X DELETE "$API/api/collections/prefs/records/$PREFROW" -H "Authorization: $SU"

TG=$(mcptext "$A" thread_get "{\"thread\":\"$NTID\"}")
chk ">>> thread_get trae la ficha: estados posibles con su grupo" \
  "$(echo "$TG" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["state_group"], any(s["group"]=="completed" for s in d["states"]))')" "completed True"
OPEN_STATE=$(echo "$TG" | python3 -c 'import sys,json;print([s["name"] for s in json.load(sys.stdin)["states"] if s["group"]!="completed"][0])')
DONE_STATE=$(echo "$TG" | python3 -c 'import sys,json;print([s["name"] for s in json.load(sys.stdin)["states"] if s["group"]=="completed"][0])')
ARG="{\"thread\":\"$NTID\",\"assignees\":[\"$BID\"],\"due\":\"2026-11-30\",\"labels\":[\"cliente\",\"portal\"],\"state\":\"$OPEN_STATE\"}"
mcptext "$A" thread_update "$ARG" >/dev/null
chk ">>> asignar, plazo, etiquetas (creadas si faltan) y reabrir, en una llamada" \
  "$(mcptext "$A" thread_get "{\"thread\":\"$NTID\"}" | python3 -c 'import sys,json
d=json.load(sys.stdin); print(d["assignees"][0]["id"]=="'"$BID"'", d["due_date"], ",".join(sorted(d["labels"])), d["state_group"]!="completed")')" "True 2026-11-30 cliente,portal True"
ARG="{\"thread\":\"$NTID\",\"assignees\":[\"zzzzzzzzzzzzzzz\"]}"
chk ">>> una persona que no existe se rechaza" \
  "$(mcptext "$A" thread_update "$ARG" | grep -c 'people_list')" 1
ARG="{\"thread\":\"$NTID\",\"state\":\"Nada\"}"
chk ">>> un estado que no existe dice cuáles hay" \
  "$(mcptext "$A" thread_update "$ARG" | grep -c 'tiene:')" 1
mcptext "$A" thread_update "{\"thread\":\"$NTID\",\"state\":\"$DONE_STATE\"}" >/dev/null
chk ">>> llevarlo a un estado completado ES terminarlo: queda el evento" \
  "$(mcptext "$A" change_list '{"project":"alpha"}' | python3 -c 'import sys,json
print(sum(1 for c in json.load(sys.stdin) if c.get("thread")=="'"$NTID"'" and c["kind"]=="thread-completed"))')" 2

T2=$(mcptext "$A" thread_create '{"project":"alpha","name":"Depende del agente"}' | j "['id']")
REL=$(mcptext "$A" thread_relate "{\"thread\":\"$NTID\",\"related\":\"$T2\",\"type\":\"blocking\"}" | j "['id']")
chk ">>> relacionar: una dependencia se dice con una relación" \
  "$(mcptext "$A" thread_get "{\"thread\":\"$T2\"}" | python3 -c 'import sys,json
r=json.load(sys.stdin)["relations"][0]; print(r["type"], r["thread"]=="'"$NTID"'")')" "blocked_by True"
ARG="{\"thread\":\"$T2\",\"related\":\"$NTID\",\"type\":\"relates_to\"}"
chk "...dos veces no" \
  "$(mcptext "$A" thread_relate "$ARG" | grep -c 'ya están relacionados')" 1
chk "...y se quita" "$(mcptext "$A" thread_unrelate "{\"relation\":\"$REL\"}" | j "['deleted']")" True
ARG="{\"thread\":\"$NTID\",\"related\":\"$T2\",\"type\":\"relates_to\"}"
chk ">>> erin no relaciona lo que no alcanza" \
  "$(mcptext "$ER" thread_relate "$ARG" | grep -c 'not found')" 1

chk ">>> change_list: lo que pasó, con nombres y si calentó" \
  "$(mcptext "$A" change_list '{}' | python3 -c 'import sys,json
c=[x for x in json.load(sys.stdin) if x.get("thread")=="'"$T2"'" and x["kind"]=="thread-created"][0]; print(c["name"], c["warms"], c["project"])')" "Depende del agente True alpha"
chk ">>> erin no ve los cambios de alpha" "$(mcptext "$ER" change_list '{"project":"alpha"}' | grep -c 'not found')" 1
chk "...ni en el total" "$(mcptext "$ER" change_list '{}')" "[]"
chk ">>> un since sin forma se rechaza" "$(mcptext "$A" change_list '{"since":"ayer"}' | grep -c 'RFC 3339')" 1

ARG="{\"thread\":\"$T2\",\"sequence\":30}"
chk ">>> sólo un lead del equipo ordena la secuencia" \
  "$(mcptext "$A" sequence_set "$ARG" | grep -c 'lead del equipo')" 1
ARG="{\"thread\":\"$T2\",\"sequence\":30}"
chk "...el lead sí" "$(mcptext "$C" sequence_set "$ARG" | j "['sequence']")" 30
mcptext "$C" sequence_set "{\"thread\":\"$T2\",\"sequence\":0}" >/dev/null
curl -s -X DELETE "$API/api/collections/threads/records/$T2" -H "Authorization: $A" >/dev/null

# A esta altura bob es el lead de alpha y alice, miembro (las pruebas de
# membresía de arriba los intercambiaron).
chk ">>> quien no es del proyecto no enlaza repositorios" \
  "$(mcptext "$ER" repo_link '{"project":"alpha","url":"github.com/cuby/portal"}' | grep -c 'not found')" 1
chk ">>> un miembro que no es lead tampoco" \
  "$(mcptext "$A" repo_link '{"project":"alpha","url":"github.com/cuby/portal"}' | grep -c 'sólo un lead')" 1
RP=$(mcptext "$B" repo_link '{"project":"alpha","url":"github.com/cuby/portal","name":"portal"}')
RPID=$(echo "$RP" | j "['id']")
chk ">>> el lead del proyecto sí, y la URL queda completa" "$(echo "$RP" | j "['url']")" "https://github.com/cuby/portal"
chk "...y repo_list trae su id para quitarlo" \
  "$(mcptext "$A" repo_list '{"project":"alpha"}' | python3 -c 'import sys,json;print(any(r["id"]=="'"$RPID"'" for r in json.load(sys.stdin)))')" True
chk ">>> una URL que no es de un repositorio se rechaza" \
  "$(mcptext "$B" repo_link '{"project":"alpha","url":"github.com"}' | grep -c 'no es la URL')" 1
chk ">>> un miembro no lo quita" "$(mcptext "$A" repo_unlink "{\"repo\":\"$RPID\"}" | grep -c 'sólo un lead')" 1
chk ">>> el lead sí" "$(mcptext "$B" repo_unlink "{\"repo\":\"$RPID\"}" | j "['deleted']")" True

ART=$(mcptext "$A" artifact_create '{"kind":"markdown","topic":"Para listar"}' | j "['id']")
chk ">>> artifact_list: mis artefactos, para retomarlos en otra sesión" \
  "$(mcptext "$A" artifact_list '{}' | python3 -c 'import sys,json;print([a["status"] for a in json.load(sys.stdin) if a["id"]=="'"$ART"'"][0])')" open
ARG="{\"id\":\"$ART\",\"pinned\":true}"
chk "...fijar uno" "$(mcptext "$A" artifact_pin "$ARG" | j "['pinned']")" True
chk "...los del equipo son sólo de sus leads" "$(mcptext "$A" artifact_list '{"all":true}' | grep -c 'lead de equipo')" 1
chk ">>> bob no borra el de alice" "$(mcptext "$B" artifact_delete "{\"id\":\"$ART\"}" | grep -c 'not found')" 1
chk ">>> borrarlo" "$(mcptext "$A" artifact_delete "{\"id\":\"$ART\"}" | j "['deleted']")" True

chk ">>> la administración se queda en la aplicación: no hay tools" \
  "$(mcp "$C" "tools/list" "{}" | python3 -c 'import sys,json
n=[t["name"] for t in json.load(sys.stdin)["result"]["tools"]]
print(sum(1 for x in n if x.startswith(("person_","people_create","token_","membership_","channel_","prompt_","inventory_access"))))')" 0

# --------------------------------------------------------- sin kanban ----
#
# El kanban se quitó entero (PLAN.md): sin columnas, sin «dónde está en el
# plan» y sin orden a mano dentro de una columna.
echo
chk ">>> ya no hay columnas de módulos" "$(code "$API/api/collections/stages/records" -H "Authorization: $C")" 404
chk "...ni de tareas" "$(code "$API/api/collections/thread_stages/records" -H "Authorization: $C")" 404
chk ">>> una burbuja ya no lleva etapa ni rango" \
  "$(curl -s "$API/api/collections/bubbles/records/$BU_P" -H "Authorization: $A" | python3 -c 'import sys,json;d=json.load(sys.stdin);print("stage" in d or "rank" in d)')" False

# ------------------------------------------------------ brief de burbuja ----
#
# Lo largo del plan va en `brief`, no en el outcome: el outcome es el contrato
# de una frase, y estirarlo hasta ser el cuaderno del plan lo habría vuelto
# ilegible en el board.
echo
LONGBRIEF=$(python3 -c "print('# Plan\\\\n\\\\n' + 'contexto. ' * 120)")
chk ">>> una burbuja lleva un brief largo, con markdown" \
  "$(code -X PATCH "$API/api/collections/bubbles/records/$BU_P" -H "Authorization: $A" -H "$JS" -d "{\"brief\":\"$LONGBRIEF\"}")" 200
chk ">>> ...y el outcome sigue siendo una frase: 500 como mucho" \
  "$(code -X PATCH "$API/api/collections/bubbles/records/$BU_P" -H "Authorization: $A" -H "$JS" -d "{\"outcome\":\"$LONGBRIEF\"}")" 400

# --------------------------------------------------- objetivo de burbuja ----
#
# Un objetivo dice PARA QUÉ sirve un cuerpo de trabajo, y el cuerpo de trabajo
# es la burbuja. Colgado de cada thread obligaba a repetir la misma respuesta en
# cada pieza y dejaba que dos piezas de lo mismo se contradijeran.
echo
OBJ2=$(post objectives "$C" '{"name":"Que el portal aguante la hora pico"}' | j "['id']")
chk ">>> una burbuja cuelga de un objetivo del departamento" \
  "$(code -X PATCH "$API/api/collections/bubbles/records/$BU_P" -H "Authorization: $A" -H "$JS" -d "{\"objective\":\"$OBJ2\"}")" 200
chk ">>> ...y el plan las cuenta por objetivo, no por thread" \
  "$(mcptext "$C" objective_list '{}' | python3 -c 'import sys,json
d=json.loads(sys.stdin.read())
print(sum(o["bubbles"] for o in d["objectives"]))')" 1
chk ">>> un thread ya no tiene objetivo propio" \
  "$(curl -s "$API/api/collections/threads/records/$T5" -H "Authorization: $A" | grep -c '"objective"')" 0

# ---------------------------------------------------------- inventario ----
#
# Dónde vive lo que hace funcionar todo esto. VERLO se asigna, y se asigna con
# una fila: es como se dice pertenecer en todo el resto de este modelo.
echo
GRP=$(post inventory_groups "$C" '{"name":"VPS","note":"máquinas que sostienen esto"}' | j "['id']")
chk ">>> el lead global levanta un grupo del inventario" "$([ -n "$GRP" ] && echo si || echo no)" si
chk ">>> ...y un lead de workspace NO: el inventario es del departamento" \
  "$(pcode inventory_groups "$A" '{"name":"Mío"}')" 400
ITM=$(post inventory_items "$C" "{\"group\":\"$GRP\",\"name\":\"vps-01\",\"provider\":\"Hetzner\",\"cost\":\"6 EUR/mes\",\"renews_at\":\"2027-01-01 00:00:00.000Z\"}" | j "['id']")
chk ">>> ...y una cosa dentro, con su proveedor y su renovación" "$([ -n "$ITM" ] && echo si || echo no)" si

chk ">>> sin asignación NO se ve: ni los grupos" \
  "$(list inventory_groups "$A" | j "['totalItems']")" 0
chk ">>> ...ni lo que hay dentro" \
  "$(list inventory_items "$A" | j "['totalItems']")" 0
# Con cero filas de acceso, "vacío ?= vacío" daba verdadero y el inventario se
# veía desde una sesión anónima — vacío, que es cuando nadie lo habría notado.
chk ">>> anónimo tampoco, ni siquiera con el inventario recién creado" \
  "$(curl -s "$API/api/collections/inventory_groups/records" | j "['totalItems']")" 0

chk ">>> asignar el acceso es escribir una FILA" \
  "$(pcode inventory_access "$C" "{\"user\":\"$AID\"}")" 200
chk ">>> ...y con ella alice ya lo ve" \
  "$(list inventory_groups "$A" | j "['totalItems']")" 1
chk ">>> ...pero bob sigue sin verlo" \
  "$(list inventory_groups "$B" | j "['totalItems']")" 0
chk ">>> nadie se asigna a sí mismo" \
  "$(pcode inventory_access "$B" "{\"user\":\"$BID\"}")" 400
chk ">>> quien mira sólo ve SU propia asignación, para poder preguntarse si le toca" \
  "$(list inventory_access "$A" | j "['totalItems']")" 1
chk ">>> y con acceso se LEE, pero no se escribe: el armario es del lead" \
  "$(pcode inventory_items "$A" "{\"group\":\"$GRP\",\"name\":\"mío\"}")" 400
# Por MCP se LEE, con la misma llave: la fila de acceso. El armario sigue siendo
# del lead, y `vault` es un enlace — este servidor no guarda secretos, así que no
# puede entregar uno por ninguna puerta.
chk ">>> un agente con acceso lee el inventario" \
  "$(mcptext "$A" inventory_list '{}' | grep -c 'vps-01')" 1
chk ">>> ...y trae dónde está la contraseña, nunca cuál es" \
  "$(mcptext "$A" inventory_list '{}' | grep -c 'password')" 0
# `ErrDenied` dice "not found", que es la misma respuesta que da la aplicación:
# quien no alcanza algo no aprende que existe.
chk ">>> sin asignación, el agente tampoco: bob no lo lee" \
  "$(mcptext "$B" inventory_list '{}' | grep -c 'not found')" 1
chk ">>> quién ve el inventario se asigna a mano: no hay tool para eso" \
  "$(mcp "$A" "tools/list" "{}" | grep -c 'inventory_access')" 0

ARG="{\"group\":\"$GRP\",\"name\":\"colado\"}"
chk ">>> escribir el inventario es del lead global" \
  "$(mcptext "$A" inventory_item_create "$ARG" | grep -c 'lead global')" 1
G2=$(mcptext "$C" inventory_group_create '{"name":"Dominios"}' | j "['id']")
IT=$(mcptext "$C" inventory_item_create "{\"group\":\"$G2\",\"name\":\"cuby.mx\",\"provider\":\"Cloudflare\",\"vault\":\"https://vault.example/cuby-mx\",\"renews_at\":\"2027-01-15\",\"notes\":\"DNS en Cloudflare\"}" | j "['id']")
chk ">>> el lead global da de alta un dominio" "$([ -n "$IT" ] && echo si || echo no)" si
chk "...y quien tiene acceso lo ve, con su grupo y su nota" \
  "$(mcptext "$A" inventory_list '{}' | python3 -c 'import sys,json
d=json.load(sys.stdin); i=[x for x in d["items"] if x["id"]=="'"$IT"'"][0]
print(i["group"], i["group_id"]=="'"$G2"'", i["renews_at"], i["note"], any(g["id"]=="'"$G2"'" for g in d["groups"]))')" "Dominios True 2027-01-15 DNS en Cloudflare True"
ARG="{\"item\":\"$IT\",\"vault\":\"hunter2\"}"
chk ">>> un vault que no es un enlace se rechaza: nunca una credencial" \
  "$(mcptext "$C" inventory_item_update "$ARG" | grep -c 'enlace http')" 1
chk ">>> un grupo con cosas no se borra" \
  "$(mcptext "$C" inventory_group_delete "{\"group\":\"$G2\"}" | grep -c 'muévelas')" 1
chk "...se borra la cosa" "$(mcptext "$C" inventory_item_delete "{\"item\":\"$IT\"}" | j "['deleted']")" True
chk "...y luego el grupo vacío" "$(mcptext "$C" inventory_group_delete "{\"group\":\"$G2\"}" | j "['deleted']")" True

curl -s -o /dev/null -X DELETE "$API/api/collections/inventory_groups/records/$GRP" -H "Authorization: $C"
chk ">>> borrar un grupo se lleva lo que había dentro" \
  "$(list inventory_items "$C" | j "['totalItems']")" 0

# ------------------------------------------------------- publicar ----
#
# Los dos scripts que deciden la versión y escriben sus notas. Se prueban por su
# CÓDIGO DE SALIDA porque así es como los usa el CI, y así es como falló el
# primer release: `notes.sh` terminaba en `[[ -n "$last" ]] && echo …`, y con la
# primera versión —que no tiene ninguna anterior— ese test es falso, el script
# salía 1 y el workflow moría sin haber tageado nada.
echo
scripts/version.sh >/dev/null 2>&1
chk ">>> version.sh sale bien aunque no haya nada que publicar" "$?" 0
scripts/notes.sh >/dev/null 2>&1
chk ">>> notes.sh sale bien aunque una sección quede vacía" "$?" 0
# Que además DIGA algo: este repositorio lleva `feat:` desde hace muchos
# commits, así que unas notas sin una sola sección serían un sed que dejó de
# reconocer la convención.
chk ">>> ...y las notas traen al menos una sección" \
  "$([ "$(scripts/notes.sh | grep -c '^### ')" -ge 1 ] && echo si || echo no)" si

# ------------------------------------------------------------- repos ----
#
# Dónde vive el CÓDIGO. Una fila por repositorio, del workspace: lo ven sus
# miembros y lo escribe su lead — la misma frase que gobierna todo lo demás que
# pertenece a un workspace.
#
# A estas alturas del guion el lead de alpha es BOB: alice lo promovió y se
# degradó a sí misma unas secciones más arriba. Los papeles aquí son los de este
# punto de la historia, no los del principio.
echo
REPO=$(post workspace_repos "$B" "{\"workspace\":\"$ALPHA\",\"url\":\"https://github.com/cuby/bubble-work\"}" | j "['id']")
chk ">>> el lead del workspace enlaza un repositorio" "$([ -n "$REPO" ] && echo si || echo no)" si
chk ">>> ...un miembro lo VE" \
  "$(list workspace_repos "$A" | j "['totalItems']")" 1
chk ">>> ...pero no enlaza ninguno" \
  "$(pcode workspace_repos "$A" "{\"workspace\":\"$ALPHA\",\"url\":\"https://github.com/alice/suyo\"}")" 400
chk ">>> el mismo repositorio dos veces se rechaza (índice único)" \
  "$(pcode workspace_repos "$B" "{\"workspace\":\"$ALPHA\",\"url\":\"https://github.com/cuby/bubble-work\"}")" 400
chk ">>> el lead global también enlaza, como en todo lo demás" \
  "$(pcode workspace_repos "$C" "{\"workspace\":\"$ALPHA\",\"url\":\"https://github.com/cuby/otro\"}")" 200
chk ">>> quien no es del workspace no ve dónde está el código" \
  "$(list workspace_repos "$ER" | j "['totalItems']")" 0
chk ">>> anónimo tampoco" \
  "$(curl -s "$API/api/collections/workspace_repos/records" | j "['totalItems']")" 0
ARG='{"project":"alpha"}'
chk ">>> un agente pregunta dónde vive el código" \
  "$(mcptext "$B" repo_list "$ARG" | grep -c 'cuby/bubble-work')" 1
chk ">>> ...y no puede enlazar ninguno: no existe la tool" \
  "$(mcp "$B" "tools/list" "{}" | grep -c 'add_repo\|link_repo')" 0
chk ">>> erin no alcanza el workspace, tampoco por esa puerta" \
  "$(mcptext "$ER" repo_list "$ARG" | grep -c 'not found')" 1

chk ">>> quitar un repositorio es del lead, no del miembro" \
  "$(code -X DELETE "$API/api/collections/workspace_repos/records/$REPO" -H "Authorization: $A")" 404
chk ">>> ...y el lead sí lo quita" \
  "$(code -X DELETE "$API/api/collections/workspace_repos/records/$REPO" -H "Authorization: $B")" 204

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

# Todo lo que se pega del portapapeles llega como `image.png`. Pisar el archivo
# cambiaba la imagen en cada documento que ya la citaba.
chk ">>> los mismos bytes con el mismo nombre reusan la ruta, sin copia" \
  "$(curl -s -X POST "$API/api/workspaces/$ALPHA/asset" -H "Authorization: $A" \
     -F "file=@$PNG" -F "name=Mi Diagrama.png" | j "['path']")" "assets/mi-diagrama.png"
PNG2=/tmp/bubble-test-2.png
printf '\x89PNG\r\n\x1a\n' > "$PNG2"; head -c 300 /dev/urandom >> "$PNG2"
chk ">>> otra imagen con el mismo nombre NO pisa la anterior" \
  "$(curl -s -X POST "$API/api/workspaces/$ALPHA/asset" -H "Authorization: $A" \
     -F "file=@$PNG2" -F "name=Mi Diagrama.png" | j "['path']")" "assets/mi-diagrama-2.png"
chk "...y la primera sigue con sus bytes" \
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

# En assets/ vive lo que se ADJUNTA a un trabajo, no sólo lo que se ve. La lista
# es blanca y corta a propósito: git guarda cada versión de cada byte para
# siempre, así que lo que entra decide cuánto crece el repositorio. Un markdown
# NO entra —ya tiene casa en docs/ o junto a su hilo— y eso lo fija la aserción
# de arriba.
PDF=/tmp/bubble-informe.pdf; printf '%%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%%%EOF\n' > "$PDF"
PDFUP=$(curl -s -X POST "$API/api/workspaces/$ALPHA/asset" -H "Authorization: $A" \
  -F "file=@$PDF" -F "name=Informe Q3.pdf")
chk ">>> un PDF sí entra, y aterriza en assets/" "$(echo "$PDFUP" | j "['path']")" "assets/informe-q3.pdf"
chk ">>> ...y el servidor dice que NO es imagen, para enlazarlo en vez de embeberlo" \
  "$(echo "$PDFUP" | j "['image']")" "False"
chk ">>> ...mientras que de una imagen dice que sí" \
  "$(curl -s -X POST "$API/api/workspaces/$ALPHA/asset" -H "Authorization: $A" \
     -F "file=@$PNG" -F "name=mi-diagrama.png" | j "['image']")" "True"
PDFURL="$API/api/workspaces/$ALPHA/file?path=assets/informe-q3.pdf"
chk ">>> se sirve con su nombre, no con file?path=…" \
  "$(curl -s -o /dev/null -D - "$PDFURL" -H "Authorization: $A" \
     | tr -d '\r' | grep -i '^content-disposition:' | tr -d ' ')" \
  'Content-Disposition:inline;filename="informe-q3.pdf"'
chk ">>> los bytes del adjunto vuelven idénticos" \
  "$(curl -s "$PDFURL" -H "Authorization: $A" | cmp -s - "$PDF" && echo si || echo no)" si
chk ">>> un adjunto es tan privado como la escritura: erin no lo ve" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$PDFURL" -H "Authorization: $ER")" 404
chk ">>> un comprimido NO entra: git lo guardaría entero en cada versión" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/workspaces/$ALPHA/asset" \
     -H "Authorization: $A" -F "file=@$PDF" -F "name=cosas.zip")" 400
chk ">>> ni un ejecutable" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/workspaces/$ALPHA/asset" \
     -H "Authorization: $A" -F "file=@$PDF" -F "name=malo.sh")" 400
rm -f "$PDF"
# El documento guarda la ruta CORTA y el servidor la resuelve al renderizar: un
# markdown con `/api/workspaces/<id>/file?path=…` dentro es un markdown que no
# sobrevive una mudanza ni se lee desde un clon de git.
IMGDOC="{\"path\":\"docs/con-imagen.md\",\"base\":\"$E0\",\"content\":\"# Con imagen\\n\\n![d](assets/mi-diagrama.png)\\n\"}"
wpatch "$A" "$IMGDOC" >/dev/null
PAGE=$(curl -s "$WSDOC?path=docs/con-imagen.md" -H "Authorization: $A")
chk ">>> el markdown conserva assets/… tal cual" \
  "$(echo "$PAGE" | python3 -c 'import sys,json;print("si" if "(assets/mi-diagrama.png)" in json.load(sys.stdin)["content"] else "no")')" si
chk ">>> ...y el html sale apuntando a la ruta que sí la sirve" \
  "$(echo "$PAGE" | python3 -c 'import sys,json;print("si" if "/api/workspaces/'"$ALPHA"'/file?path=assets/mi-diagrama.png" in json.load(sys.stdin)["html"] else "no")')" si

# `/api/markdown` reescribe SÓLO si le dicen de qué workspace es. Quien lo llame
# sin workspace —un comentario, por ejemplo— recibe la ruta corta intacta, y el
# navegador pide algo que no existe. Es un contrato, no un detalle: se prueba en
# los dos sentidos.
MDBODY='{"content":"![d](assets/mi-diagrama.png)","workspace":"'"$ALPHA"'"}'
chk ">>> /api/markdown con workspace resuelve la imagen" \
  "$(curl -s -X POST "$API/api/markdown" -H "Authorization: $A" -H 'Content-Type: application/json' \
     -d "$MDBODY" | python3 -c 'import sys,json;print("si" if "/api/workspaces/'"$ALPHA"'/file?path=assets/mi-diagrama.png" in json.load(sys.stdin)["html"] else "no")')" si
chk ">>> ...y sin workspace la deja corta, que es por qué hay que mandarlo" \
  "$(curl -s -X POST "$API/api/markdown" -H "Authorization: $A" -H 'Content-Type: application/json' \
     -d '{"content":"![d](assets/mi-diagrama.png)"}' | python3 -c 'import sys,json;print("si" if "\"assets/mi-diagrama.png\"" in json.load(sys.stdin)["html"] else "no")')" si

chk ">>> el árbol lista la imagen" \
  "$(curl -s "$API/api/workspaces/$ALPHA/tree" -H "Authorization: $A" | python3 -c 'import sys,json
e=json.load(sys.stdin)["entries"]
print(next((x["area"] for x in e if x["path"]=="assets/mi-diagrama.png"), "falta"))')" asset
chk "y quedó commiteada como todo lo demás" \
  "$(cd "$R/alpha" && git log --oneline -- assets/mi-diagrama.png | wc -l | tr -d ' ')" 1
rm -f "$PNG" "$BIG"

# ------------------------------------------------- tokens de agente ----
echo
tpost(){ curl -s -X POST "$API/api/tokens$2" -H "Authorization: $1" -H "$JS" -d "$3"; }
tcode(){ code -X POST "$API/api/tokens$2" -H "Authorization: $1" -H "$JS" -d "$3"; }
NEWTOK=$(tpost "$A" "" '{"name":"agente de alice","days":0}')
TOK=$(echo "$NEWTOK" | j "['token']")
TOKID=$(echo "$NEWTOK" | j "['id']")
chk ">>> alice genera un token de agente, y se le enseña una vez" "${TOK:0:3}" "bw_"
chk "...sin caducidad si se pide así" "$(echo "$NEWTOK" | j "['expires_at']")" ""
chk ">>> con él, un agente entra al MCP como alice" \
  "$(mcptext "Bearer $TOK" project_list '{}' | grep -c '"alpha"')" 1
chk ">>> la huella no sale nunca por la API" \
  "$(list agent_tokens "$A" | python3 -c 'import sys,json;d=json.load(sys.stdin);print("si" if d["totalItems"]==1 and "hash" not in d["items"][0] else "no")')" si
chk ">>> bob no ve el token de alice" "$(list agent_tokens "$B" | j "['totalItems']")" 0
chk "...el lead global sí, para poder revocarlo" "$(list agent_tokens "$C" | j "['totalItems']")" 1
chk ">>> el token abre el MCP y nada más: la API REST no lo acepta" \
  "$(code "$API/api/limits" -H "Authorization: Bearer $TOK")" 401
chk ">>> nadie crea un token por la API de registros: la huella la calcula el servidor" \
  "$(pcode agent_tokens "$A" "{\"owner\":\"$AID\",\"name\":\"x\",\"hash\":\"abc\"}")" 403
chk ">>> caducidad negativa se rechaza" "$(tcode "$A" "" '{"name":"x","days":-1}')" 400
chk ">>> bob no extiende el token de alice" "$(tcode "$B" "/$TOKID/extend" '{"days":30}')" 404
chk ">>> alice lo extiende 30 días" \
  "$(tpost "$A" "/$TOKID/extend" '{"days":30}' | python3 -c 'import sys,json;print("si" if json.load(sys.stdin)["expires_at"] else "no")')" si
chk "...y le quita la caducidad con 0" "$(tpost "$A" "/$TOKID/extend" '{"days":0}' | j "['expires_at']")" ""
chk ">>> y lo renombra" "$(tpost "$A" "/$TOKID/rename" '{"name":"claude de alice"}' | j "['name']")" "claude de alice"
chk ">>> bob no lo revoca" "$(tcode "$B" "/$TOKID/revoke" '{}')" 404
chk ">>> el lead global sí" \
  "$(tpost "$C" "/$TOKID/revoke" '{}' | python3 -c 'import sys,json;print("si" if json.load(sys.stdin)["revoked_at"] else "no")')" si
chk "...y revocado ya no entra al MCP" "$(code -X POST "$API/mcp" -H "Authorization: Bearer $TOK" -H "$JS" \
  -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}')" 401
chk "...ni se extiende: se genera otro" "$(tcode "$A" "/$TOKID/extend" '{"days":30}')" 400
chk ">>> un token inventado tampoco entra" "$(code -X POST "$API/mcp" -H "Authorization: Bearer bw_inventado" -H "$JS" \
  -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}')" 401

# ------------------------------------------ personas: alta y borrado lógico ----
echo
ppost(){ curl -s -X POST "$API/api/people$2" -H "Authorization: $1" -H "$JS" -d "$3"; }
pplcode(){ code -X POST "$API/api/people$2" -H "Authorization: $1" -H "$JS" -d "$3"; }
chk ">>> un member no da de alta a nadie" \
  "$(pplcode "$A" "" '{"email":"frank@bubble.test","password":"passwordpass"}')" 403
FRANK=$(ppost "$C" "" '{"email":"frank@bubble.test","name":"Frank","password":"passwordpass","role":"member"}' | j "['id']")
chk ">>> el lead global da de alta a frank" "$([ -n "$FRANK" ] && echo si || echo no)" si
FR=$(login frank@bubble.test)
chk "...y frank entra" "$([ -n "$FR" ] && echo si || echo no)" si
chk "...el mismo correo dos veces, no" \
  "$(pplcode "$C" "" '{"email":"frank@bubble.test","password":"passwordpass"}')" 400
FTOK=$(tpost "$FR" "" '{"name":"agente de frank","days":90}' | j "['token']")
chk "frank tiene un agente" "$(mcp "Bearer $FTOK" tools/list '{}' | grep -q '"project_list"' && echo si || echo no)" si
chk ">>> nadie se marca borrado editando un usuario" \
  "$(code -X PATCH "$API/api/collections/users/records/$FRANK" -H "Authorization: $C" -H "$JS" -d '{"deleted_at":"2026-01-01 00:00:00.000Z"}')" 404
chk ">>> ni se borra de verdad por la API" \
  "$(code -X DELETE "$API/api/collections/users/records/$AID" -H "Authorization: $A")" 403
chk ">>> un member no borra a nadie" "$(pplcode "$A" "/$FRANK/delete" '{}')" 403
chk ">>> el lead global no se borra a sí mismo" "$(pplcode "$C" "/$CID/delete" '{}')" 400
chk ">>> el lead global borra a frank" \
  "$(ppost "$C" "/$FRANK/delete" '{}' | python3 -c 'import sys,json;print("si" if json.load(sys.stdin)["deleted_at"] else "no")')" si
chk "...frank ya no entra" "$(code -X POST "$API/api/collections/users/auth-with-password" -H "$JS" \
  -d '{"identity":"frank@bubble.test","password":"passwordpass"}')" 403
chk "...su sesión abierta murió" "$(code "$API/api/limits" -H "Authorization: $FR")" 401
chk "...y su agente tampoco entra" "$(code -X POST "$API/mcp" -H "Authorization: Bearer $FTOK" -H "$JS" \
  -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}')" 401
chk ">>> restaurarlo lo devuelve" "$(ppost "$C" "/$FRANK/restore" '{}' | j "['deleted_at']")" ""
chk "...frank entra otra vez" "$([ -n "$(login frank@bubble.test)" ] && echo si || echo no)" si
chk "...y su agente también" "$(mcp "Bearer $FTOK" tools/list '{}' | grep -q '"project_list"' && echo si || echo no)" si
chk ">>> el lead global no se quita el rol a sí mismo" \
  "$(code -X PATCH "$API/api/collections/users/records/$CID" -H "Authorization: $C" -H "$JS" -d '{"role":"member"}')" 400
chk ">>> con dos leads, uno le quita el rol al otro" \
  "$(code -X PATCH "$API/api/collections/users/records/$DAVE" -H "Authorization: $C" -H "$JS" -d '{"role":"member"}')" 200
chk ">>> y al último no se lo quita nadie, ni un superuser" \
  "$(code -X PATCH "$API/api/collections/users/records/$CID" -H "Authorization: $SU" -H "$JS" -d '{"role":"member"}')" 400
chk ">>> una persona pone su avatar" \
  "$(printf '\x89PNG\r\n\x1a\n' > /tmp/bubble-avatar.png; head -c 200 /dev/urandom >> /tmp/bubble-avatar.png
     code -X PATCH "$API/api/collections/users/records/$AID" -H "Authorization: $A" -F "avatar=@/tmp/bubble-avatar.png;type=image/png")" 200
chk "...pero no el de otra" \
  "$(code -X PATCH "$API/api/collections/users/records/$BID" -H "Authorization: $A" -F "avatar=@/tmp/bubble-avatar.png;type=image/png")" 404
rm -f /tmp/bubble-avatar.png

# ------------------------------- prioridad del hilo y secuencia de ejecución ----
echo
# Sin burbuja: la de alpha se borró más arriba, y lo que se prueba aquí es el hilo.
SQ1=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Paso uno\"}" | j "['id']")
SQ2=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Paso uno bis\"}" | j "['id']")
SQ3=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Paso dos\",\"assignees\":[\"$AID\"]}" | j "['id']")
tpatch(){ code -X PATCH "$API/api/collections/threads/records/$2" -H "Authorization: $1" -H "$JS" -d "$3"; }
FEAT=$(list features "$A" | j "['items'][0]['id']")
chk ">>> la secuencia nace apagada" "$(list features "$A" | j "['items'][0]['sequence']")" False
chk "...y next lo dice en vez de inventar una línea" "$(mcptext "$A" sequence_next '{}' | j "['enabled']")" False
chk ">>> un member no la enciende" \
  "$(code -X PATCH "$API/api/collections/features/records/$FEAT" -H "Authorization: $A" -H "$JS" -d '{"sequence":true}')" 404
chk ">>> el lead global sí" \
  "$(code -X PATCH "$API/api/collections/features/records/$FEAT" -H "Authorization: $C" -H "$JS" -d '{"sequence":true}')" 200
chk ">>> un member no pone la prioridad de un hilo" "$(tpatch "$A" "$SQ1" '{"priority":"P1"}')" 404
chk ">>> ni lo mete en la secuencia" "$(tpatch "$A" "$SQ1" '{"sequence":10}')" 404
chk "...pero sigue pudiendo renombrarlo" "$(tpatch "$A" "$SQ1" '{"name":"Paso uno"}')" 200
chk ">>> un member tampoco crea un hilo ya priorizado" \
  "$(pcode threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"colado\",\"priority\":\"P1\"}")" 400
chk ">>> el lead global pone la prioridad del hilo" "$(tpatch "$C" "$SQ1" '{"priority":"P1"}')" 200
chk "...que vive separada de la de su burbuja" \
  "$(curl -s "$API/api/workspaces/$ALPHA/board" -H "Authorization: $A" | python3 -c "import sys,json;d=json.load(sys.stdin);print(next(t['priority'] for t in d['threads'] if t['id']=='$SQ1'))")" P1
chk ">>> por MCP, un member no la pone" \
  "$(mcptext "$A" thread_update "{\"thread\":\"$SQ2\",\"priority\":\"P2\"}" | grep -c 'only the global lead')" 1
chk "...el lead global sí" "$(mcptext "$C" thread_update "{\"thread\":\"$SQ2\",\"priority\":\"P2\"}" | j "['priority']")" P2
chk ">>> una prioridad que no existe se rechaza" \
  "$(mcptext "$C" thread_update "{\"thread\":\"$SQ2\",\"priority\":\"P9\"}" | grep -c 'P1, P2, P3 or P4')" 1
tpatch "$C" "$SQ1" '{"sequence":10}' >/dev/null
tpatch "$C" "$SQ2" '{"sequence":10}' >/dev/null
tpatch "$C" "$SQ3" '{"sequence":20}' >/dev/null
chk ">>> el lead arma la secuencia, y el board la devuelve" \
  "$(curl -s "$API/api/workspaces/$ALPHA/board" -H "Authorization: $A" | python3 -c "import sys,json;d=json.load(sys.stdin);print(next(t['sequence'] for t in d['threads'] if t['id']=='$SQ3'))")" 20
NEXT=$(mcptext "$A" sequence_next '{}')
chk ">>> next: ahora van dos hilos en paralelo" "$(echo "$NEXT" | python3 -c 'import sys,json;print(len(json.load(sys.stdin)["now"]["threads"]))')" 2
chk "...luego el paso dos" "$(echo "$NEXT" | python3 -c 'import sys,json;print(json.load(sys.stdin)["then"]["threads"][0]["name"])')" "Paso dos"
chk "...y lo mío es el paso dos" "$(echo "$NEXT" | python3 -c 'import sys,json;print(json.load(sys.stdin)["mine"]["name"])')" "Paso dos"
chk ">>> erin no ve la secuencia de un proyecto que no alcanza" \
  "$(mcptext "$ER" sequence_next '{}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["now"])')" None
OWNB=$(post bubbles "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"De bob\",\"owners\":[\"$BID\"]}" | j "['id']")
SQ4=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"bubble\":\"$OWNB\",\"name\":\"Sin asignar de bob\"}" | j "['id']")
tpatch "$C" "$SQ4" '{"sequence":30}' >/dev/null
chk ">>> lo mío incluye lo sin asignar de una burbuja a mi cargo" \
  "$(mcptext "$B" sequence_next '{}' | python3 -c 'import sys,json;print((json.load(sys.stdin)["mine"] or {}).get("name"))')" "Sin asignar de bob"
chk "...pero no a quien no responde por ella" \
  "$(mcptext "$A" sequence_next '{}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["mine"]["name"])')" "Paso dos"
chk ">>> terminar un hilo por REST" "$(code -X POST "$API/api/threads/$SQ1/complete" -H "Authorization: $A")" 200
chk "...con uno de los dos abierto, el paso sigue siendo ahora" \
  "$(mcptext "$A" sequence_next '{}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["now"]["sequence"])')" 10
code -X POST "$API/api/threads/$SQ2/complete" -H "Authorization: $A" >/dev/null
chk ">>> terminado el paso, la secuencia se recorre sola" \
  "$(mcptext "$A" sequence_next '{}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["now"]["threads"][0]["name"])')" "Paso dos"
chk ">>> el board dice en qué paso vivo va cada hilo: el que quedó es ahora el 1" \
  "$(curl -s "$API/api/workspaces/$ALPHA/board" -H "Authorization: $A" | python3 -c "import sys,json;d=json.load(sys.stdin);print(next(t.get('step',0) for t in d['threads'] if t['id']=='$SQ3'))")" 1
chk "...y uno terminado ya no tiene paso" \
  "$(curl -s "$API/api/workspaces/$ALPHA/board" -H "Authorization: $A" | python3 -c "import sys,json;d=json.load(sys.stdin);print(next(t.get('step',0) for t in d['threads'] if t['id']=='$SQ1'))")" 0

# ------------------- las dos columnas laterales de la secuencia, paginadas ----
#
# Lo que está fuera de la línea y lo que ya se terminó crecen sin techo, así que
# se piden por páginas. Lo terminado se pregunta por el ESTADO y no por
# `completed_at`: esa columna existe en el esquema y no la escribe nadie.
Q="$API/api/queue"
echo
chk ">>> la cola pide un kind válido" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$Q?kind=cosas&state=open" -H "Authorization: $A")" 400
chk ">>> ...y un state válido" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$Q?kind=threads&state=quizá" -H "Authorization: $A")" 400
chk ">>> anónimo no la lee" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$Q?kind=threads&state=open")" 401
chk ">>> erin, que no está en ningún proyecto, recibe una página vacía y no un error" \
  "$(curl -s "$Q?kind=threads&state=open" -H "Authorization: $ER" | j "['items']")" "[]"
chk ">>> la cola abierta trae hilos fuera de la línea" \
  "$(curl -s "$Q?kind=threads&state=open" -H "Authorization: $C" | python3 -c 'import sys,json
d=json.load(sys.stdin)
print("si" if d["items"] and all(not i.get("sequence") for i in d["items"]) else "no")')" si
chk ">>> ...lo más reciente primero" \
  "$(curl -s "$Q?kind=threads&state=open" -H "Authorization: $C" | python3 -c 'import sys,json
c=[i["created"] for i in json.load(sys.stdin)["items"]]
print("si" if c == sorted(c, reverse=True) else "no")')" si
chk ">>> ...y con lo justo para dibujar la tarjeta" \
  "$(curl -s "$Q?kind=threads&state=open" -H "Authorization: $C" | python3 -c 'import sys,json
i=json.load(sys.stdin)["items"][0]
print("si" if {"id","name","workspace","created"} <= set(i) else "no")')" si
chk ">>> una página de uno dice que hay más" \
  "$(curl -s "$Q?kind=threads&state=open&perPage=1" -H "Authorization: $C" | j "['more']")" True
P1=$(curl -s "$Q?kind=threads&state=open&perPage=1" -H "Authorization: $C" | j "['items'][0]['id']")
P2=$(curl -s "$Q?kind=threads&state=open&perPage=1&page=2" -H "Authorization: $C" | j "['items'][0]['id']")
chk ">>> ...y la segunda página no repite la primera" \
  "$([ -n "$P1" ] && [ -n "$P2" ] && [ "$P1" != "$P2" ] && echo si || echo no)" si
chk ">>> la cola de módulos existe igual" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$Q?kind=bubbles&state=done" -H "Authorization: $C")" 200
chk ">>> una página demasiado lejos se rechaza en vez de recorrer la tabla" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$Q?kind=threads&state=open&page=9999" -H "Authorization: $C")" 400

# ----------------------------------- revisado: lo hecho que ya se miró ----
#
# Entre «terminado» y «borrado» faltaba el gesto de quien mira lo hecho y dice
# que está bien. Es lo que convierte la columna derecha de la secuencia en una
# cola —se vacía— en vez de un historial que sólo crece.
DONEQ="$Q?kind=bubbles&state=done"
echo
RVB=$(curl -s -X POST "$API/api/collections/bubbles/records" -H "Authorization: $C" \
  -H 'Content-Type: application/json' \
  -d "{\"workspace\":\"$ALPHA\",\"name\":\"Para revisar\",\"closed_at\":\"2026-09-01 00:00:00.000Z\"}" | j "['id']")
chk ">>> lo cerrado sale en la cola de hecho" \
  "$(curl -s "$DONEQ" -H "Authorization: $C" | python3 -c 'import sys,json
print("si" if any(i["id"]=="'"$RVB"'" for i in json.load(sys.stdin)["items"]) else "no")')" si
chk ">>> marcarlo revisado es editar la fila, no un rol aparte" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$API/api/collections/bubbles/records/$RVB" \
     -H "Authorization: $A" -H 'Content-Type: application/json' \
     -d '{"reviewed_at":"2026-09-02 00:00:00.000Z"}')" 200
chk ">>> ...y entonces sale de la cola: la columna se vacía" \
  "$(curl -s "$DONEQ" -H "Authorization: $C" | python3 -c 'import sys,json
print("si" if any(i["id"]=="'"$RVB"'" for i in json.load(sys.stdin)["items"]) else "no")')" no
chk ">>> desmarcarlo la devuelve" \
  "$(curl -s -X PATCH "$API/api/collections/bubbles/records/$RVB" -H "Authorization: $A" \
     -H 'Content-Type: application/json' -d '{"reviewed_at":""}' >/dev/null; \
     curl -s "$DONEQ" -H "Authorization: $C" | python3 -c 'import sys,json
print("si" if any(i["id"]=="'"$RVB"'" for i in json.load(sys.stdin)["items"]) else "no")')" si
chk ">>> reabrir un módulo lo saca de lo hecho" \
  "$(curl -s -X PATCH "$API/api/collections/bubbles/records/$RVB" -H "Authorization: $A" \
     -H 'Content-Type: application/json' -d '{"closed_at":"","closure":""}' >/dev/null; \
     curl -s "$DONEQ" -H "Authorization: $C" | python3 -c 'import sys,json
print("si" if any(i["id"]=="'"$RVB"'" for i in json.load(sys.stdin)["items"]) else "no")')" no

# ----------------------------- la línea de MÓDULOS, hermana de la de hilos ----
#
# Dos líneas independientes a propósito: una contesta «¿qué cuerpo de trabajo
# atacamos antes?» y la otra «¿qué pieza se hace antes?». Y `next` sigue siendo
# de hilos: lo que un agente puede empezar y terminar es una pieza.
BUBS="$API/api/collections/bubbles/records"
echo
# El par que prueba la guarda: alice SÍ alcanza esta burbuja —lo demuestra
# editando su outcome— y aun así no puede escribir su paso. Sin la primera
# línea, el 404 de la segunda podría ser sólo que no la ve.
chk ">>> un lead de workspace alcanza la burbuja" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$BUBS/$NBUB" -H "Authorization: $A" \
     -H 'Content-Type: application/json' -d '{"outcome":"nadie escribe dos veces"}')" 200
# 404 y no 403: con `sequence` en el cuerpo la fila deja de pasar su propia
# regla, así que para quien pide no existe. Es como contesta el resto de la API.
chk ">>> ...y aun así no ordena la línea de módulos: cruza proyectos" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$BUBS/$NBUB" -H "Authorization: $A" \
     -H 'Content-Type: application/json' -d '{"sequence":10}')" 404
chk ">>> el lead global sí" \
  "$(curl -s -X PATCH "$BUBS/$NBUB" -H "Authorization: $C" -H 'Content-Type: application/json' \
     -d '{"sequence":10}' | j "['sequence']")" 10
chk ">>> ...y el board la devuelve" \
  "$(curl -s "$API/api/board" -H "Authorization: $C" | python3 -c 'import sys,json
b=json.load(sys.stdin)["bubbles"]
print(next((x.get("sequence",0) for x in b if x["id"]=="'"$NBUB"'"), "falta"))')" 10
# La independencia, fijada: el paso del módulo NO es el de sus hilos.
chk ">>> las dos líneas son independientes: el hilo conserva el suyo" \
  "$(curl -s "$API/api/collections/threads/records/$NTID" -H "Authorization: $A" | j "['sequence']")" 0

# ------------------- lo que pasa en una fecha y NO es trabajo ----
#
# Una junta no se completa, no produce evidencia y NO calienta. Por eso es una
# fila suya y no un hilo con fecha: un hilo que renaciera cada lunes emitiría
# `thread-created` todas las semanas y mantendría una burbuja caliente sin que
# nadie trabajara.
CEV="$API/api/collections/calendar_events/records"
echo
chk ">>> un lead de workspace NO pone una fecha del calendario del equipo" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$CEV" -H "Authorization: $A" \
     -H 'Content-Type: application/json' \
     -d '{"name":"Junta","start":"2026-10-05 00:00:00.000Z","repeat":"weekly"}')" 403
EVBEFORE=$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")
CEID=$(curl -s -X POST "$CEV" -H "Authorization: $C" -H 'Content-Type: application/json' \
  -d '{"name":"Junta semanal","start":"2026-10-05 00:00:00.000Z","repeat":"weekly"}' | j "['id']")
chk ">>> el lead global sí, y con su repetición" \
  "$(curl -s "$CEV/$CEID" -H "Authorization: $A" | j "['repeat']")" weekly
chk ">>> ...y la ve cualquiera que trabaje aquí" \
  "$(curl -s "$CEV/$CEID" -H "Authorization: $B" | j "['name']")" "Junta semanal"
chk ">>> anónimo no" "$(curl -s "$CEV?perPage=1" | j "['totalItems']")" 0
chk ">>> una repetición que no está en el menú se rechaza" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$CEV" -H "Authorization: $C" \
     -H 'Content-Type: application/json' \
     -d '{"name":"Cada tercer jueves","start":"2026-10-05 00:00:00.000Z","repeat":"rrule"}')" 400
# La aserción que sostiene todo el argumento.
chk ">>> crear una fecha NO deja evidencia: no calienta nada" \
  "$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")" "$EVBEFORE"
# 404 y no 403: la regla de borrado no lo alcanza, así que para él esa fila no
# existe como algo borrable. Es como contesta el resto de la API.
chk ">>> un member no la borra" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$CEV/$CEID" -H "Authorization: $B")" 404
chk ">>> el lead global sí" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$CEV/$CEID" -H "Authorization: $C")" 204

# ------------------------------------- las preferencias de quien mira ----
#
# Casi todo lo de vista se queda en el navegador. Esto es para lo que tiene que
# seguir a la persona entre dispositivos, y por eso es una fila — con la regla
# que hace que sea SUYA.
PREFS="$API/api/collections/prefs/records"
echo
chk ">>> sin fila, no hay preferencias, y eso no es un error" \
  "$(curl -s "$PREFS?perPage=1" -H "Authorization: $A" | j "['totalItems']")" 0
PREFID=$(curl -s -X POST "$PREFS" -H "Authorization: $A" -H 'Content-Type: application/json' \
  -d "{\"user\":\"$AID\",\"value\":{\"planner\":\"tasks\"}}" | j "['id']")
chk ">>> alice guarda cómo quiere ver el planeador" "$(test -n "$PREFID" && echo si || echo no)" si
chk ">>> ...y la lee de vuelta" \
  "$(curl -s "$PREFS?perPage=1" -H "Authorization: $A" | j "['items'][0]['value']['planner']")" tasks
chk ">>> bob NO ve la fila de alice" \
  "$(curl -s "$PREFS?perPage=1" -H "Authorization: $B" | j "['totalItems']")" 0
chk ">>> ...ni por id" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$PREFS/$PREFID" -H "Authorization: $B")" 404
chk ">>> ni el lead global: no hay ninguna razón para leer la vista de otro" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$PREFS/$PREFID" -H "Authorization: $C")" 404
chk ">>> bob no le escribe las preferencias a alice" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$PREFS" -H "Authorization: $B" \
     -H 'Content-Type: application/json' -d "{\"user\":\"$AID\",\"value\":{}}")" 400
# 404 y no 403: tras el cambio la fila ya no pasaría su propia regla, así que
# para quien pide deja de existir. Es la misma forma en que el resto de la API
# contesta a «eso no es tuyo».
chk ">>> ...y alice no regala la suya cambiándole el dueño" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$PREFS/$PREFID" -H "Authorization: $A" \
     -H 'Content-Type: application/json' -d "{\"user\":\"$BID\"}")" 404
chk ">>> una segunda fila para la misma persona se rechaza" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$PREFS" -H "Authorization: $A" \
     -H 'Content-Type: application/json' -d "{\"user\":\"$AID\",\"value\":{}}")" 400
chk ">>> guardar otra vez sobreescribe el saco" \
  "$(curl -s -X PATCH "$PREFS/$PREFID" -H "Authorization: $A" -H 'Content-Type: application/json' \
     -d '{"value":{"planner":"bubbles"}}' | j "['value']['planner']")" bubbles
PINID=$(echo "$T1" | j "['id']")
PINPATCH=$(curl -s -X PATCH "$PREFS/$PREFID" -H "Authorization: $A" -H 'Content-Type: application/json' \
    -d "{\"value\":{\"planner\":\"bubbles\",\"thread_pins\":[\"$PINID\"]}}")
chk ">>> un pin de hilo se guarda en las preferencias personales" \
  "$(echo "$PINPATCH" | j "['value']['thread_pins'][0]")" "$PINID"
PINREAD=$(curl -s "$PREFS/$PREFID" -H "Authorization: $A")
PINSAVED=$(echo "$PINREAD" | j "['value']['thread_pins'][0]")
if [ "$PINSAVED" != "$PINID" ]; then echo "  pin read response: $PINREAD"; fi
chk "...y persiste al leerlo de nuevo" "$PINSAVED" "$PINID"
chk ">>> los pins de alice no se publican ni al lead global" \
  "$(code "$PREFS/$PREFID" -H "Authorization: $C")" 404
# 403 y no 404: la fila existe y se puede ver, lo que no existe es la puerta.
chk ">>> nadie borra una fila de preferencias: se sobreescribe" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$PREFS/$PREFID" -H "Authorization: $A")" 403
chk ">>> anónimo no las ve" \
  "$(curl -s "$PREFS?perPage=1" | j "['totalItems']")" 0

# ------------------------------------------------ canales del inbox ----
echo
chk ">>> un member no ve los canales" "$(code "$API/api/channels" -H "Authorization: $A")" 403
chk ">>> el lead global sí, y WhatsApp viene apagado" \
  "$(curl -s "$API/api/channels" -H "Authorization: $C" | python3 -c 'import sys,json;d={x["kind"]:x for x in json.load(sys.stdin)};print(d["whatsapp"]["state"])')" disabled
chk ">>> un member no lo enciende" "$(code -X POST "$API/api/channels/whatsapp/enable" -H "Authorization: $A")" 403
chk ">>> encendido sin cuenta vinculada, pide vincular" \
  "$(curl -s -X POST "$API/api/channels/whatsapp/enable" -H "Authorization: $C" | j "['state']")" unlinked
chk "...sin QR que escanear todavía" "$(code "$API/api/channels/whatsapp/qr.png" -H "Authorization: $C")" 404
chk "...y sin cuenta no hay grupos que elegir" \
  "$(curl -s -X POST "$API/api/channels/whatsapp/groups" -H "Authorization: $C" | j "['message']" | grep -ci 'vincula')" 1
chk ">>> un canal que el binario no trae no existe" "$(code -X POST "$API/api/channels/telegrama/enable" -H "Authorization: $C")" 404
chk ">>> nadie lee la tabla de canales por la API" "$(code "$API/api/collections/channels/records" -H "Authorization: $C")" 403
chk ">>> apagarlo" "$(curl -s -X POST "$API/api/channels/whatsapp/disable" -H "Authorization: $C" | j "['state']")" disabled


# ------------------------------- artefactos efímeros: el interrogatorio ----
# Lo que se piensa antes de que exista el trabajo. Cuelga de una nota del inbox,
# lo escribe el servidor, sólo lo ven quien lo abrió y el lead global, y no
# calienta nada. Un agente pregunta en rondas; la persona contesta en la app.
echo
EV0=$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")
GR=$(mcptext "$A" artifact_create '{"kind":"grill","topic":"Facturación por cliente"}')
GID=$(echo "$GR" | j "['id']")
chk ">>> un agente abre un interrogatorio sobre un tema" "$(echo "$GR" | j "['status']")" open
chk "...y crea la nota del inbox de la que cuelga, a nombre de su persona" \
  "$(curl -s "$API/api/collections/inbox_items/records/$(echo "$GR" | j "['inbox_item']")" -H "Authorization: $A" | j "['captured_by']")" "$AID"
chk "...y devuelve la dirección donde se contesta" "$(echo "$GR" | j "['url']" | grep -c "/artefacto/$GID")" 1
chk ">>> un kind que no existe se rechaza" \
  "$(mcptext "$A" artifact_create '{"kind":"brainstorm","topic":"x"}' | grep -c 'no artifact kind')" 1

R1='{"id":"'$GID'","entries":[
 {"type":"question","data":{"id":"Q1","round":1,"title":"¿Por cliente?","body":"…","recommendation":"sí"}},
 {"type":"question","data":{"id":"Q2","round":1,"title":"¿Moneda?","body":"…","recommendation":"MXN"}},
 {"type":"question","data":{"id":"Q3","round":2,"title":"¿Plantilla?","body":"…","recommendation":"una","depends_on":["Q1","Q2"]}}]}'
AP1=$(mcptext "$A" artifact_append "$R1")
chk ">>> el agente publica una ronda entera de una vez" "$(echo "$AP1" | j "['appended']")" 3
WC=$(echo "$AP1" | j "['watch_command']")
chk "...y recibe el comando que espera a la persona" "$(echo "$WC" | grep -c "/api/artifacts/$GID/wait?key=")" 1
WKEY=$(echo "$WC" | sed 's/.*key=\([A-Za-z0-9]*\).*/\1/')
chk ">>> la llave no sale por la API de registros" \
  "$(curl -s "$API/api/collections/artifacts/records/$GID" -H "Authorization: $A" | grep -c watch_key)" 0
chk ">>> sin la llave, la espera no existe" "$(code "$API/api/artifacts/$GID/wait?key=nope")" 404
chk ">>> sin respuestas nuevas, la espera vence vacía (204)" "$(code "$API/api/artifacts/$GID/wait?key=$WKEY&since=3&timeout=1")" 204
chk ">>> artifact_wait por MCP dice que sigue esperando" \
  "$(mcptext "$A" artifact_wait '{"id":"'$GID'","since":3,"timeout":1}' | j "['waiting']")" True
chk ">>> una pregunta no se reescribe" \
  "$(mcptext "$A" artifact_append '{"id":"'$GID'","entries":[{"type":"question","data":{"id":"Q1","round":1,"title":"t","body":"b"}}]}' | grep -c 'already exists')" 1
chk ">>> depender de algo que nunca se preguntó se rechaza" \
  "$(mcptext "$A" artifact_append '{"id":"'$GID'","entries":[{"type":"question","data":{"id":"Q9","round":2,"title":"t","body":"b","depends_on":["Q7"]}}]}' | grep -c 'never asked')" 1
chk ">>> la frontera se deriva: lo que depende de algo abierto espera" \
  "$(mcptext "$A" artifact_read '{"id":"'$GID'"}' | python3 -c 'import sys,json;print(",".join(q["state"] for q in json.load(sys.stdin)["questions"]))')" "frontier,frontier,blocked"

chk ">>> la app recibe cada pregunta renderizada con el motor de los documentos" \
  "$(curl -s "$API/api/artifacts/$GID" -H "Authorization: $A" | python3 -c 'import sys,json;print(json.load(sys.stdin)["questions"][0]["body_html"].startswith("<p>"))')" True
chk "...y el agente recibe su markdown, sin una copia en HTML" \
  "$(mcptext "$A" artifact_read '{"id":"'$GID'"}' | grep -c '_html')" 0
chk ">>> bob no ve el interrogatorio de alice" "$(code "$API/api/artifacts/$GID" -H "Authorization: $B")" 404
chk "...ni por la API de registros" "$(code "$API/api/collections/artifacts/records/$GID" -H "Authorization: $B")" 404
chk "...ni por MCP" "$(mcptext "$B" artifact_read '{"id":"'$GID'"}' | grep -c 'not found')" 1
chk ">>> el lead global sí" "$(code "$API/api/artifacts/$GID" -H "Authorization: $C")" 200
chk ">>> nadie escribe una entrada por la API de registros" \
  "$(pcode artifact_entries "$A" '{"artifact":"'$GID'","seq":99,"type":"answer","author":"'$AID'","via":"app"}')" 403

# El script de verdad, en segundo plano, como lo correría un agente.
WOUT=$(mktemp); bash -c "$WC" > "$WOUT" 2>&1 & WPID=$!
sleep 1.5
chk ">>> mientras el script espera, la app dice que el agente espera" \
  "$(curl -s "$API/api/artifacts/$GID" -H "Authorization: $A" | j "['watched']")" True
ANS=$(curl -s -X POST "$API/api/artifacts/$GID/answers" -H "Authorization: $A" -H "$JS" \
  -d '{"answers":[{"question":"Q1","verdict":"accepted"},{"question":"Q2","verdict":"override","text":"USD también"}]}')
chk ">>> la persona contesta en la app" "$(echo "$ANS" | j "['answered']")" 2
sleep 2
chk ">>> el script del agente se entera: UNA línea, con lo respondido" \
  "$(grep -c '^respondidas Q1,Q2 .*seq=5$' "$WOUT")" 1
chk "...y lo que dependía de eso pasa a la frontera" \
  "$(echo "$ANS" | python3 -c 'import sys,json;print([q["state"] for q in json.load(sys.stdin)["questions"] if q["id"]=="Q3"][0])')" frontier
chk "...con su texto cuando no acepta la recomendación" \
  "$(echo "$ANS" | python3 -c 'import sys,json;print([q["answer"]["text"] for q in json.load(sys.stdin)["questions"] if q["id"]=="Q2"][0])')" "USD también"
chk ">>> desde la app sólo se contesta: preguntar es del agente" \
  "$(code -X POST "$API/api/artifacts/$GID/answers" -H "Authorization: $A" -H "$JS" -d '{"answers":[{"question":"Q1","verdict":"maybe"}]}')" 400
chk ">>> lo que se contestó en la terminal lo anota el agente" \
  "$(mcptext "$A" artifact_append '{"id":"'$GID'","entries":[{"type":"answer","data":{"question":"Q3","verdict":"accepted"}}]}' | j "['appended']")" 1
chk "...marcado como tal" \
  "$(mcptext "$A" artifact_read '{"id":"'$GID'"}' | python3 -c 'import sys,json;print([q["answer"]["via"] for q in json.load(sys.stdin)["questions"] if q["id"]=="Q3"][0])')" harness

LIST=$(curl -s "$API/api/artifacts" -H "Authorization: $A")
chk ">>> la caja lista los míos, con su progreso" "$(echo "$LIST" | j "['artifacts'][0]['answered']")" 3
chk "...y dice si mi agente está conectado" "$(echo "$LIST" | j "['agent']['live']")" False
chk "...y bob no ve ninguno" "$(curl -s "$API/api/artifacts" -H "Authorization: $B" | j "['artifacts']")" "[]"
chk ">>> el inbox del lead incluye artefactos del equipo para mostrar su TTL" \
  "$(curl -s "$API/api/artifacts?scope=inbox" -H "Authorization: $C" | python3 -c 'import sys,json; print(any(a["id"] == "'$GID'" and a.get("expires_at") for a in json.load(sys.stdin)["artifacts"]))')" True
chk "...sin cambiar la caja personal del lead" \
  "$(curl -s "$API/api/artifacts" -H "Authorization: $C" | j "['artifacts']")" "[]"
chk ">>> un miembro no puede pedir los artefactos del inbox departamental" \
  "$(code "$API/api/artifacts?scope=inbox" -H "Authorization: $B")" 403
chk "...caduca a los 30 días si nadie lo fija" "$(echo "$LIST" | j "['artifacts'][0]['expires_at']" | grep -c .)" 1
chk ">>> fijarlo le quita la caducidad" \
  "$(curl -s -X POST "$API/api/artifacts/$GID/pin" -H "Authorization: $A" -H "$JS" -d '{"pinned":true}' | j "['expires_at']")" ""
chk ">>> bob no lo fija" "$(code -X POST "$API/api/artifacts/$GID/pin" -H "Authorization: $B" -H "$JS" -d '{"pinned":true}')" 404

chk ">>> el traspaso necesita adónde fue el trabajo" \
  "$(mcptext "$A" artifact_handoff '{"id":"'$GID'","targets":[]}' | grep -c 'at least one')" 1
HO=$(mcptext "$A" artifact_handoff '{"id":"'$GID'","targets":[{"kind":"thread","id":"'$T1ID'"}]}')
chk ">>> el traspaso lo cierra" "$(echo "$HO" | j "['status']")" handed_off
sleep 2
chk ">>> ...y el script del agente termina solo" "$(kill -0 $WPID 2>/dev/null && echo vivo || echo terminó)" terminó
chk "...diciendo fin" "$(tail -1 "$WOUT" | cut -c1-3)" fin
kill $WPID 2>/dev/null; rm -f "$WOUT"
chk "...y la nota queda triada al hilo" \
  "$(curl -s "$API/api/collections/inbox_items/records/$(echo "$HO" | j "['inbox_item']")" -H "Authorization: $A" | j "['thread']")" "$T1ID"
chk ">>> traspasado, ya no se escribe" \
  "$(mcptext "$A" artifact_append '{"id":"'$GID'","entries":[{"type":"note","data":{"text":"x"}}]}' | grep -c 'read-only')" 1
chk ">>> interrogar NO calienta nada: ningún evento" \
  "$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")" "$EV0"
chk ">>> agents_md enseña párrafos sin hard-wrap a quien opera" \
  "$(mcptext "$A" agents_md '{}' | grep -c 'no hagas hard-wrap')" 1
chk ">>> ...y también al planner" \
  "$(mcptext "$C" agents_md '{}' | grep -c 'no hagas hard-wrap')" 1
chk ">>> el protocolo de interrogatorio llega al crear uno" \
  "$(mcptool "$A" artifact_create '{"kind":"grill","topic":"protocolo"}' | python3 -c 'import sys,json
c=json.load(sys.stdin)["result"]["content"]
print(len(c), c[1]["text"].splitlines()[0], "url" in json.loads(c[0]["text"]))')" "2 # Interrogatorio a través de Bubble True"

GR2=$(mcptext "$A" artifact_create '{"kind":"grill","topic":"otra"}'); NOTE2=$(echo "$GR2" | j "['inbox_item']")
curl -s -X DELETE "$API/api/collections/inbox_items/records/$NOTE2" -H "Authorization: $A" >/dev/null
chk ">>> borrar la nota se lleva sus artefactos" "$(code "$API/api/artifacts/$(echo "$GR2" | j "['id']")" -H "Authorization: $A")" 404


# --------------------------------------- markdown artifacts / read plan ----
echo
MDART=$(mcptext "$A" artifact_create '{"kind":"markdown","topic":"Informe temporal"}')
MDID=$(echo "$MDART" | j "['id']"); MDNOTE=$(echo "$MDART" | j "['inbox_item']")
chk ">>> el agente abre un Markdown efímero" "$(echo "$MDART" | j "['kind']")" markdown
chk "...no propone watcher: no espera respuestas" "$(echo "$MDART" | j "['watch_command']")" ""
MDEV=$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")
chk ">>> publica la primera revisión" \
  "$(mcptext "$A" artifact_append '{"id":"'$MDID'","entries":[{"type":"document","data":{"filename":"informe.md","markdown":"# Informe\n\n- [ ] ejemplo\n","base":0}}]}' | j "['appended']")" 1
chk ">>> la web usa el renderer compartido" \
  "$(curl -s "$API/api/artifacts/$MDID" -H "Authorization: $A" | python3 -c 'import json,sys;d=json.load(sys.stdin);print("<h1" in d["document"]["html"] and "checkbox" in d["document"]["html"])')" True
chk "...MCP recibe fuente vigente aunque since no traiga entries" \
  "$(mcptext "$A" artifact_read '{"id":"'$MDID'","since":99}' | j "['document']['seq']")" 1
chk ">>> no acepta sobrescritura con base vieja" \
  "$(mcptext "$A" artifact_append '{"id":"'$MDID'","entries":[{"type":"document","data":{"filename":"informe.md","markdown":"pisado","base":0}}]}' | grep -c 'document changed')" 1
chk ">>> no permite publicar desde la web" \
  "$(code -X POST "$API/api/artifacts/$MDID/answers" -H "Authorization: $A" -H "$JS" -d '{"answers":[{"question":"Q1","verdict":"accepted"}]}')" 404
chk ">>> nadie publica revisiones por la API de registros" \
  "$(pcode artifact_entries "$A" '{"artifact":"'$MDID'","seq":99,"type":"document","author":"'$AID'","via":"harness"}')" 403
chk ">>> una revisión nueva usa su base actual" \
  "$(mcptext "$A" artifact_append '{"id":"'$MDID'","entries":[{"type":"document","data":{"filename":"informe.md","markdown":"## Definitivo\nTexto nuevo","base":1}}]}' | j "['appended']")" 1
chk ">>> lote con segunda base inválida revierte la primera" \
  "$(mcptext "$A" artifact_append '{"id":"'$MDID'","entries":[{"type":"document","data":{"filename":"informe.md","markdown":"provisional","base":2}},{"type":"document","data":{"filename":"informe.md","markdown":"incorrecto","base":2}}]}' | grep -c 'document changed')" 1
chk "...sigue en la revisión 2" "$(mcptext "$A" artifact_read '{"id":"'$MDID'"}' | j "['document']['seq']")" 2
chk ">>> bob tampoco lee Markdown ajeno" "$(code "$API/api/artifacts/$MDID" -H "Authorization: $B")" 404
chk "...su captura es visible en el Inbox de sólo lectura del creador" \
  "$(curl -s "$API/api/planner/read" -H "Authorization: $A" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(any(n["id"]=="'$MDNOTE'" for n in d["inbox"]) and any(a["id"]=="'$MDID'" for a in d["artifacts"]))')" True
chk ">>> el Inbox del operador no revela capturas ajenas" \
  "$(curl -s "$API/api/planner/read" -H "Authorization: $B" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(all(n["captured_by"]=="'$BID'" for n in d["inbox"]) and all(a["creator"]=="'$BID'" for a in d["artifacts"]))')" True
mcptext "$A" artifact_handoff '{"id":"'$MDID'","targets":[{"kind":"thread","id":"'$T1ID'"}]}' >/dev/null
chk ">>> el documento traspasado conserva su captura navegable hasta caducar" \
  "$(curl -s "$API/api/planner/read" -H "Authorization: $A" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(any(n["id"]=="'$MDNOTE'" for n in d["inbox"]))')" True
chk ">>> publicar revisiones no produce calor" \
  "$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")" "$MDEV"
chk ">>> el protocolo de documentos llega al crear uno" \
  "$(mcptool "$A" artifact_create '{"kind":"markdown","topic":"protocolo md"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["result"]["content"][1]["text"].splitlines()[0])')" "# Documentos Markdown efímeros"

# ----------------------------------------------- mi espacio: privado ----
# Notas y archivos personales. Privado incluso frente al lead global, y fuera
# del calor: no es trabajo del departamento.
PEV=$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")
PN=$(post personal_notes "$A" '{"owner":"'$AID'","title":"Pendientes","body":"- [ ] uno","color":"#bcecf5","workspace":"'$ALPHA'"}')
PNID=$(echo "$PN" | j "['id']")
chk ">>> alice crea una nota en su espacio" "$(echo "$PN" | j "['title']")" Pendientes
chk ">>> nadie crea notas a nombre de otro" \
  "$(pcode personal_notes "$B" '{"owner":"'$AID'","title":"x"}')" 400
chk ">>> ni las asocia a un proyecto donde no participa" \
  "$(pcode personal_notes "$A" '{"owner":"'$AID'","title":"x","workspace":"'$BETA'"}')" 400
chk ">>> un color que no es #rrggbb se rechaza" \
  "$(pcode personal_notes "$A" '{"owner":"'$AID'","title":"x","color":"red; x"}')" 400
chk ">>> bob no la ve" "$(code "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $B")" 404
chk ">>> el lead global tampoco" "$(code "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $C")" 404
chk "...ni en el listado" "$(list personal_notes "$C" | j "['totalItems']")" 0
chk ">>> el lead global no la edita" \
  "$(code -X PATCH "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $C" -H "$JS" -d '{"title":"mía"}')" 404
chk ">>> alice no la regala cambiando el dueño" \
  "$(code -X PATCH "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $A" -H "$JS" -d '{"owner":"'$BID'"}')" 404
chk ">>> alice marca su casilla" \
  "$(curl -s -X PATCH "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $A" -H "$JS" -d '{"body":"- [x] uno"}' | j "['body']")" "- [x] uno"
chk ">>> el post-it guarda su lugar en el lienzo" \
  "$(curl -s -X PATCH "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $A" -H "$JS" -d '{"x":-420,"y":135,"w":300}' | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["x"],d["y"],d["w"])')" "-420 135 300"
chk "...y su tamaño: ancho hasta 1200 y alto elegido" \
  "$(curl -s -X PATCH "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $A" -H "$JS" -d '{"w":1000,"h":420}' | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["w"],d["h"])')" "1000 420"
chk "...un alto fuera de rango se rechaza" \
  "$(code -X PATCH "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $A" -H "$JS" -d '{"h":5000}')" 400
chk "...un ancho fuera de rango se rechaza" \
  "$(code -X PATCH "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $A" -H "$JS" -d '{"w":5000}')" 400
chk "...y el lead global no se la mueve" \
  "$(code -X PATCH "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $C" -H "$JS" -d '{"x":0}')" 404
PFILE=$(mktemp); printf 'secreto personal' > "$PFILE"
PA=$(curl -s -X POST "$API/api/collections/personal_assets/records" -H "Authorization: $A" \
  -F "owner=$AID" -F "note=$PNID" -F "name=plan.txt" -F "file=@$PFILE;filename=plan.txt")
PAID=$(echo "$PA" | j "['id']"); PAFILE=$(echo "$PA" | j "['file']")
chk ">>> alice adjunta un archivo a su nota" "$(echo "$PA" | j "['name']")" plan.txt
chk ">>> nadie adjunta a una nota ajena" \
  "$(code -X POST "$API/api/collections/personal_assets/records" -H "Authorization: $B" -F "owner=$BID" -F "note=$PNID" -F "name=x" -F "file=@$PFILE;filename=x.txt")" 400
chk ">>> el archivo no se descarga sin token" "$(code "$API/api/files/personal_assets/$PAID/$PAFILE")" 404
TOKA=$(curl -s -X POST "$API/api/files/token" -H "Authorization: $A" | j "['token']")
TOKC=$(curl -s -X POST "$API/api/files/token" -H "Authorization: $C" | j "['token']")
chk "...con el token de alice sí" "$(curl -s "$API/api/files/personal_assets/$PAID/$PAFILE?token=$TOKA")" "secreto personal"
chk "...con el del lead global no" "$(code "$API/api/files/personal_assets/$PAID/$PAFILE?token=$TOKC")" 404
chk ">>> el espacio no produce calor ni eventos" \
  "$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")" "$PEV"
chk ">>> borrar la nota se lleva sus archivos" \
  "$(code -X DELETE "$API/api/collections/personal_notes/records/$PNID" -H "Authorization: $A"; code "$API/api/collections/personal_assets/records/$PAID" -H "Authorization: $A")" 204404
rm -f "$PFILE"
PREFROW=$(curl -s "$API/api/collections/prefs/records?perPage=1" -H "Authorization: $A" | j "['items'][0]['id']")
chk ">>> los colores de proyecto viven en las preferencias personales" \
  "$(curl -s -X PATCH "$API/api/collections/prefs/records/$PREFROW" -H "Authorization: $A" -H "$JS" \
     -d "$(curl -s "$API/api/collections/prefs/records/$PREFROW" -H "Authorization: $A" | python3 -c 'import sys,json;v=json.load(sys.stdin)["value"] or {};v["project_colors"]={sys.argv[1]:"#bcecf5"};print(json.dumps({"value":v}))' "$ALPHA")" \
     | j "['value']['project_colors']['$ALPHA']")" "#bcecf5"
chk ">>> el markdown compartido subraya con ++texto++" \
  "$(curl -s -X POST "$API/api/markdown" -H "Authorization: $A" -H "$JS" -d '{"content":"un ++dato++"}' | j "['html']" | grep -c '<u>dato</u>')" 1

# --------------------------------------- borrar sólo lo efímero ----
chk ">>> borrar un artefacto exige sesión" "$(code -X DELETE "$API/api/artifacts/$MDID")" 401
chk ">>> bob no borra artefactos de alice" "$(code -X DELETE "$API/api/artifacts/$MDID" -H "Authorization: $B")" 404
chk "...y el documento sigue publicado" "$(code "$API/api/artifacts/$MDID" -H "Authorization: $A")" 200
chk ">>> borrar uno inexistente tampoco filtra información" "$(code -X DELETE "$API/api/artifacts/xxxxxxxxxxxxxxx" -H "Authorization: $A")" 404
chk ">>> los registros crudos siguen sin puerta de borrado" \
  "$(code -X DELETE "$API/api/collections/artifacts/records/$MDID" -H "Authorization: $A")" 403
DELEV=$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")
DELDOC=$(curl -s "$API/api/threads/$T1ID/document" -H "Authorization: $A" | j "['hash']")
curl -s -X POST "$API/api/artifacts/$MDID/pin" -H "Authorization: $A" -H "$JS" -d '{"pinned":true}' >/dev/null
chk ">>> el lead global puede borrar un Markdown ajeno, fijado y traspasado" \
  "$(code -X DELETE "$API/api/artifacts/$MDID" -H "Authorization: $C")" 204
chk "...ya no se puede leer" "$(code "$API/api/artifacts/$MDID" -H "Authorization: $A")" 404
chk "...no quedan revisiones huérfanas" \
  "$(curl -sG "$API/api/collections/artifact_entries/records" -H "Authorization: $SU" --data-urlencode "filter=artifact=\"$MDID\"" | j "['totalItems']")" 0
chk "...la nota conserva su hilo de destino" \
  "$(curl -s "$API/api/collections/inbox_items/records/$MDNOTE" -H "Authorization: $A" | j "['thread']")" "$T1ID"
chk "...el documento duradero sigue intacto" \
  "$(curl -s "$API/api/threads/$T1ID/document" -H "Authorization: $A" | j "['hash']")" "$DELDOC"
chk ">>> el creador también borra un grill traspasado" \
  "$(code -X DELETE "$API/api/artifacts/$GID" -H "Authorization: $A")" 204

DELGR=$(mcptext "$A" artifact_create '{"kind":"grill","topic":"Grill descartable"}')
DELID=$(echo "$DELGR" | j "['id']"); DELNOTE=$(echo "$DELGR" | j "['inbox_item']")
DELAP=$(mcptext "$A" artifact_append '{"id":"'$DELID'","entries":[{"type":"question","data":{"id":"Q1","round":1,"title":"¿Seguir?","body":"Prueba","recommendation":"sí"}}]}')
DELWC=$(echo "$DELAP" | j "['watch_command']")
DELOUT=$(mktemp); bash -c "$DELWC" > "$DELOUT" 2>&1 & DELPID=$!
sleep 0.3
chk ">>> el creador borra un grill abierto" "$(code -X DELETE "$API/api/artifacts/$DELID" -H "Authorization: $A")" 204
for _ in {1..30}; do kill -0 "$DELPID" 2>/dev/null || break; sleep 0.1; done
chk "...el watcher termina, no reintenta para siempre" "$(kill -0 "$DELPID" 2>/dev/null && echo running || echo stopped)" stopped
chk "...y dice fin una sola vez" "$(grep -c '^fin:' "$DELOUT")" 1
kill "$DELPID" 2>/dev/null; wait "$DELPID" 2>/dev/null; rm -f "$DELOUT"
chk "...se conservó su nota del Inbox" "$(code "$API/api/collections/inbox_items/records/$DELNOTE" -H "Authorization: $A")" 200
chk "...se borraron sus preguntas" \
  "$(curl -sG "$API/api/collections/artifact_entries/records" -H "Authorization: $SU" --data-urlencode "filter=artifact=\"$DELID\"" | j "['totalItems']")" 0
chk ">>> borrar artefactos no produce calor ni eventos" \
  "$(curl -s "$API/api/collections/events/records?perPage=1" -H "Authorization: $SU" | j "['totalItems']")" "$DELEV"

RP_OWN=$(post bubbles "$C" '{"workspace":"'$ALPHA'","name":"RP own","owners":["'$BID'"],"due_date":"2026-12-20"}' | j "['id']")
RP_ASSIGNED=$(post bubbles "$C" '{"workspace":"'$ALPHA'","name":"RP assigned","owners":["'$AID'"]}' | j "['id']")
RP_HIDE=$(post bubbles "$C" '{"workspace":"'$ALPHA'","name":"RP hidden","owners":["'$AID'"]}' | j "['id']")
RP_CHILD=$(post threads "$C" '{"workspace":"'$ALPHA'","bubble":"'$RP_OWN'","name":"RP owned child","assignees":["'$AID'"]}' | j "['id']")
RP_MINE=$(post threads "$C" '{"workspace":"'$ALPHA'","bubble":"'$RP_ASSIGNED'","name":"RP assigned child","assignees":["'$BID'"]}' | j "['id']")
RP_SIBLING=$(post threads "$C" '{"workspace":"'$ALPHA'","bubble":"'$RP_ASSIGNED'","name":"RP hidden sibling","assignees":["'$AID'"]}' | j "['id']")
RP_STAND=$(post threads "$C" '{"workspace":"'$ALPHA'","name":"RP standalone","assignees":["'$BID'"]}' | j "['id']")
# An assignment must never bypass the outer workspace-membership boundary.
RP_OTHER_WS=$(post workspaces "$C" '{"name":"RP inaccessible","slug":"rp-inaccessible"}' | j "['id']")
# Asignado por el superuser, que no invita a nadie: un hilo con bob asignado en un
# proyecto del que bob NO es miembro (por la puerta de la aplicación, asignarlo
# lo habría sumado al proyecto).
RP_OTHER=$(post threads "$SU" '{"workspace":"'$RP_OTHER_WS'","name":"RP outside membership","assignees":["'$BID'"]}' | j "['id']")
RP_READ=$(curl -s "$API/api/planner/read" -H "Authorization: $B")
chk ">>> responsable o asignado ve sus burbujas, no las otras del mismo workspace" \
  "$(echo "$RP_READ" | python3 -c 'import json,sys;d=json.load(sys.stdin);ids={b["id"] for b in d["board"]["bubbles"]};print("'$RP_OWN'" in ids and "'$RP_ASSIGNED'" in ids and "'$RP_HIDE'" not in ids)')" True
chk ">>> ve hijos del módulo propio, asignados y sueltos; no hermanos ajenos" \
  "$(echo "$RP_READ" | python3 -c 'import json,sys;d=json.load(sys.stdin);ids={t["id"] for t in d["board"]["threads"]};print(all(i in ids for i in ["'$RP_CHILD'","'$RP_MINE'","'$RP_STAND'"]) and "'$RP_SIBLING'" not in ids and "'$RP_OTHER'" not in ids)')" True
chk ">>> tampoco expone nombres de proyectos fuera del alcance" \
  "$(echo "$RP_READ" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(not any(w["id"]=="'$RP_OTHER_WS'" for w in d["board"]["workspaces"]))')" True
chk "...incluye vencimiento del módulo participante" \
  "$(echo "$RP_READ" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(next(b for b in d["board"]["bubbles"] if b["id"]=="'$RP_OWN'")["due_date"][:10])')" 2026-12-20
chk ">>> el lead conserva el alcance departamental" \
  "$(curl -s "$API/api/planner/read" -H "Authorization: $C" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(any(b["id"]=="'$RP_HIDE'" for b in d["board"]["bubbles"]))')" True
chk ">>> el planeador de lectura exige sesión" "$(code "$API/api/planner/read")" 401

# ------------------------------------------------------------- prompts ----
# Todo lo que se le dice a la IA, editable por el lead: los AGENTS.md, los
# documentos del binario y la descripción de cada tool. Lo embebido queda como
# el original.
echo
chk ">>> la lista trae los AGENTS.md, el sistema y las tools" \
  "$(curl -s "$API/api/prompts" -H "Authorization: $A" | python3 -c 'import sys,json;d=json.load(sys.stdin);g={x["group"] for x in d};print(sorted(g), any(x["key"]=="tool/document_edit" for x in d), any(x["key"] in ("guide","house_rules") for x in d))')" "['agentes', 'sistema', 'tools'] True False"
chk ">>> un member los lee" "$(code "$API/api/prompt?key=agents_guide" -H "Authorization: $A")" 200
chk "...menos el AGENTS.md del planner, que es del lead" "$(code "$API/api/prompt?key=agents/planner" -H "Authorization: $A")" 404
chk "...pero no los edita" \
  "$(code -X POST "$API/api/prompt" -H "Authorization: $A" -H "$JS" -d '{"key":"agents_guide","content":"x"}')" 403
chk ">>> el lead edita la descripción de una tool" \
  "$(curl -s -X POST "$API/api/prompt" -H "Authorization: $C" -H "$JS" -d '{"key":"tool/thread_history","content":"EDITADA: qué le pasó al hilo"}' | j "['edited']")" True
chk "...y el agente la ve en la siguiente llamada" \
  "$(mcp "$A" "tools/list" "{}" | python3 -c 'import sys,json;print([t["description"] for t in json.load(sys.stdin)["result"]["tools"] if t["name"]=="thread_history"][0])')" "EDITADA: qué le pasó al hilo"
chk ">>> restaurar vuelve al original del binario" \
  "$(curl -s -X POST "$API/api/prompt" -H "Authorization: $C" -H "$JS" -d '{"key":"tool/thread_history","reset":true}' | j "['edited']")" False
chk "...y el agente también" \
  "$(mcp "$A" "tools/list" "{}" | python3 -c 'import sys,json;print([t["description"] for t in json.load(sys.stdin)["result"]["tools"] if t["name"]=="thread_history"][0].startswith("EDITADA"))')" False
chk ">>> la guía editada es la que sirve agents_md" \
  "$(curl -s -X POST "$API/api/prompt" -H "Authorization: $C" -H "$JS" -d '{"key":"agents_guide","content":"# Guía editada"}' >/dev/null; mcptext "$A" agents_md '{}' | sed -n 3p)" "# Guía editada"
chk ">>> ...y la tabla de tools sigue: se genera, no se edita" \
  "$(mcptext "$A" agents_md '{}' | grep -c '^## Tools por categoría')" 1
curl -s -X POST "$API/api/prompt" -H "Authorization: $C" -H "$JS" -d '{"key":"agents_guide","reset":true}' >/dev/null
chk ">>> un prompt vacío se rechaza: hay que restaurar" \
  "$(code -X POST "$API/api/prompt" -H "Authorization: $C" -H "$JS" -d '{"key":"grill","content":"  "}')" 400
chk ">>> el AGENTS.md se guarda por la misma puerta" \
  "$(curl -s -X POST "$API/api/prompt" -H "Authorization: $C" -H "$JS" -d '{"key":"agents/operators","content":"# Operadores v2"}' | j "['content']")" "# Operadores v2"
chk ">>> nadie escribe una versión por la API de registros" \
  "$(pcode prompt_versions "$C" '{"key":"agents_guide","content":"x","author":"'$CID'"}')" 403


# --------------------------------- el superuser: opera la caja, no trabaja ----
echo
# El checkbox se dirige por su posición de fuente, no por orden visual.
# Un ejemplo cercado no produce un input, pero ocupa un índice del motor actual.
INLINEID=$(post threads "$A" "{\"workspace\":\"$ALPHA\",\"name\":\"Inline checkbox regression\"}" | j "['id']")
INLINEURL="$API/api/threads/$INLINEID/document"
INLINEBASE=$(curl -s "$INLINEURL" -H "Authorization: $A" | j "['hash']")
INLINEWRITE=$(curl -s -X PATCH "$INLINEURL" -H "Authorization: $A" -H "$JS" \
  -d "{\"base\":\"$INLINEBASE\",\"content\":\"\u0060\u0060\u0060md\\n- [ ] ejemplo\\n\u0060\u0060\u0060\\n\\n- [ ] real\\n\"}")
chk ">>> el render direcciona el checkbox real después del ejemplo" \
  "$(echo "$INLINEWRITE" | j "['html']" | grep -c 'data-todo-index=\"1\"')" 1
INLINEBASE=$(echo "$INLINEWRITE" | j "['hash']")
INLINEWRITE=$(curl -s -X PATCH "$INLINEURL" -H "Authorization: $A" -H "$JS" \
  -d "{\"base\":\"$INLINEBASE\",\"todo\":{\"index\":1,\"done\":true}}")
chk ">>> el tick desde el render persiste y no cambia el ejemplo" \
  "$(curl -s "$INLINEURL" -H "Authorization: $A" | j "['content']" | python3 -c 'import sys;s=sys.stdin.read();print("si" if "- [ ] ejemplo" in s and "- [x] real" in s else "no")')" si
curl -s -X DELETE "$API/api/collections/threads/records/$INLINEID" -H "Authorization: $A" >/dev/null

# ------------------------------------------------ equipos: contextos aislados ----
# Un segundo lead global con su equipo PERSONAL. Lo de TI lo ven los dos; lo
# personal de cada uno, sólo su dueño. Invitar a alguien a un PROYECTO le enseña
# ese proyecto y nada más del equipo.
echo
GINA=$(ppost "$C" "" '{"email":"gina@bubble.test","name":"Gina","password":"passwordpass","role":"lead"}' | j "['id']")
GI=$(login gina@bubble.test)
chk ">>> una persona nueva entra a TI como miembro" \
  "$(curl -s "$API/api/collections/team_members/records?filter=$(python3 -c "import urllib.parse;print(urllib.parse.quote(\"user='$GINA'\"))")" -H "Authorization: $SU" | j "['items'][0]['team']")" "$TI"
chk ">>> un lead global es lead implícito de TI" \
  "$(curl -s "$API/api/teams" -H "Authorization: $GI" | python3 -c 'import sys,json;t=[x for x in json.load(sys.stdin)["teams"] if x["id"]=="'"$TI"'"][0];print(t["role"])')" lead
chk ">>> un member no crea un equipo personal" \
  "$(code -X POST "$API/api/teams/personal" -H "Authorization: $A" -H "$JS" -d '{}')" 403
GP=$(curl -s "$API/api/teams" -H "Authorization: $GI" | python3 -c 'import sys,json;print(next((t["id"] for t in json.load(sys.stdin)["teams"] if t["personal"]),""))')
chk ">>> un lead global nuevo nace con su equipo personal" "$([ -n "$GP" ] && echo si || echo no)" si
chk "...a su nombre" "$(curl -s "$API/api/teams" -H "Authorization: $GI" | python3 -c 'import sys,json;print([t["name"] for t in json.load(sys.stdin)["teams"] if t["personal"]][0])')" "Personal · Gina"
chk "...uno solo" "$(code -X POST "$API/api/teams/personal" -H "Authorization: $GI" -H "$JS" -d '{}')" 400
chk ">>> su equipo por defecto sigue siendo el compartido" "$(curl -s "$API/api/teams" -H "Authorization: $GI" | j "['default']")" "$TI"
HANK=$(ppost "$C" "" '{"email":"hank@bubble.test","name":"Hank","password":"passwordpass","role":"member"}' | j "['id']")
HK=$(login hank@bubble.test)
chk ">>> un member nuevo no tiene equipo personal" \
  "$(curl -s "$API/api/collections/teams/records?filter=$(python3 -c "import urllib.parse;print(urllib.parse.quote(\"owner='$HANK'\"))")" -H "Authorization: $SU" | j "['totalItems']")" 0
ARG='{"role":"lead"}'
curl -s -o /dev/null -X PATCH "$API/api/collections/users/records/$HANK" -H "Authorization: $C" -H "$JS" -d "$ARG"
chk ">>> al ascenderlo a lead global recibe el suyo" \
  "$(curl -s "$API/api/collections/teams/records?filter=$(python3 -c "import urllib.parse;print(urllib.parse.quote(\"owner='$HANK' && personal=true\"))")" -H "Authorization: $SU" | j "['totalItems']")" 1
chk ">>> carol, también lead global, NO ve el equipo personal de gina" \
  "$(curl -s "$API/api/teams" -H "Authorization: $C" | python3 -c 'import sys,json;print(any(t["id"]=="'"$GP"'" for t in json.load(sys.stdin)["teams"]))')" False
chk "...ni por la colección" "$(code "$API/api/collections/teams/records/$GP" -H "Authorization: $C")" 404
ARG="{\"name\":\"Plan de dirección\",\"team\":\"$GP\"}"
GWS=$(mcptext "$GI" project_create "$ARG" | j "['id']")
chk ">>> gina crea un proyecto en su equipo personal" \
  "$(curl -s "$API/api/collections/workspaces/records/$GWS" -H "Authorization: $GI" | j "['team']")" "$GP"
ARG="{\"project\":\"$GWS\",\"name\":\"Presupuesto 2027\"}"
GT=$(mcptext "$GI" thread_create "$ARG" | j "['id']")
chk ">>> carol no ve el proyecto: ni en la lista" \
  "$(curl -s "$API/api/collections/workspaces/records?perPage=200" -H "Authorization: $C" | python3 -c 'import sys,json;print(any(w["id"]=="'"$GWS"'" for w in json.load(sys.stdin)["items"]))')" False
chk "...ni por id" "$(code "$API/api/collections/workspaces/records/$GWS" -H "Authorization: $C")" 404
chk "...ni su tablero" "$(code "$API/api/workspaces/$GWS/board" -H "Authorization: $C")" 404
chk "...ni sus hilos" "$(code "$API/api/collections/threads/records/$GT" -H "Authorization: $C")" 404
chk "...ni el documento" "$(code "$API/api/threads/$GT/document" -H "Authorization: $C")" 404
chk "...ni en el tablero de todos" \
  "$(curl -s "$API/api/board" -H "Authorization: $C" | python3 -c 'import sys,json;print(any(w["id"]=="'"$GWS"'" for w in json.load(sys.stdin)["workspaces"]))')" False
ARG="{\"thread\":\"$GT\"}"
chk "...ni por MCP" "$(mcptext "$C" thread_get "$ARG" | grep -c 'not found')" 1
chk "...ni en lo que cambió" \
  "$(mcptext "$C" change_list '{}' | python3 -c 'import sys,json;print(any(c.get("thread")=="'"$GT"'" for c in json.load(sys.stdin)))')" False
chk ">>> gina sí ve TI, con lo de carol" \
  "$(curl -s "$API/api/collections/workspaces/records/$ALPHA" -H "Authorization: $GI" | j "['id']")" "$ALPHA"
ARG="{\"name\":\"Colado\",\"team\":\"$GP\"}"
chk ">>> carol no mete un proyecto en el equipo personal de gina" \
  "$(mcptext "$C" project_create "$ARG" | grep -c 'no ves ningún equipo')" 1

GWSM=$(curl -s -X POST "$API/api/collections/memberships/records" -H "Authorization: $GI" -H "$JS" \
  -d "{\"workspace\":\"$GWS\",\"user\":\"$BID\",\"role\":\"member\"}" | j "['id']")
chk ">>> gina invita a bob a SU proyecto" "$([ -n "$GWSM" ] && echo si || echo no)" si
chk "...y bob lo ve" "$(code "$API/api/collections/workspaces/records/$GWS" -H "Authorization: $B")" 200
chk "...pero no el equipo personal" \
  "$(curl -s "$API/api/teams" -H "Authorization: $B" | python3 -c 'import sys,json;print(any(t["id"]=="'"$GP"'" for t in json.load(sys.stdin)["teams"]))')" False
ARG="{\"team\":\"$GP\",\"note\":\"colada\"}"
chk "...y no le mete ideas" \
  "$(mcptext "$B" inbox_capture "$ARG" | grep -c 'no ves ningún equipo')" 1
ARG="{\"user\":\"$BID\",\"role\":\"member\"}"
chk ">>> a un equipo personal no se agrega a nadie" \
  "$(curl -s -X POST "$API/api/teams/$GP/members" -H "Authorization: $GI" -H "$JS" -d "$ARG" | j "['message']" | grep -c 'invita a la persona a un PROYECTO')" 1

ARG="{\"team\":\"$GP\",\"note\":\"Reestructurar el área\"}"
GIDEA=$(mcptext "$GI" inbox_capture "$ARG" | j "['id']")
chk ">>> las ideas de gina en su equipo personal: carol no las ve" \
  "$(mcptext "$C" inbox_list '{}' | python3 -c 'import sys,json;print(any(x["id"]=="'"$GIDEA"'" for x in json.load(sys.stdin)))')" False
chk "...ni por la colección" "$(code "$API/api/collections/inbox_items/records/$GIDEA" -H "Authorization: $C")" 404
CIDEA=$(mcptext "$C" inbox_capture '{"note":"Idea de TI"}' | j "['id']")
ARG="{\"team\":\"$TI\"}"
chk ">>> una idea de TI sí la ven los dos leads de TI" \
  "$(mcptext "$GI" inbox_list "$ARG" | python3 -c 'import sys,json;print(any(x["id"]=="'"$CIDEA"'" for x in json.load(sys.stdin)))')" True
ARG="{\"idea\":\"$CIDEA\",\"thread\":\"$GT\"}"
chk ">>> una idea de TI no se vuelve trabajo en un proyecto de otro equipo" \
  "$(mcptext "$GI" inbox_triage "$ARG" | grep -c 'de otro equipo')" 1
curl -s -o /dev/null -X DELETE "$API/api/collections/inbox_items/records/$CIDEA" -H "Authorization: $C"

ARG="{\"team\":\"$GP\",\"name\":\"Rentabilidad del área\"}"
GOBJ=$(mcptext "$GI" objective_create "$ARG" | j "['id']")
chk ">>> los objetivos son del equipo: carol no ve los de gina" \
  "$(mcptext "$C" objective_list '{}' | python3 -c 'import sys,json;print(any(o["id"]=="'"$GOBJ"'" for o in json.load(sys.stdin)["objectives"]))')" False
TIB=$(mcptext "$C" bubble_create '{"project":"alpha","name":"Módulo de TI"}' | j "['id']")
ARG="{\"bubble\":\"$TIB\",\"objective\":\"$GOBJ\"}"
chk ">>> un módulo de TI no cuelga de un objetivo de otro equipo" \
  "$(mcptext "$GI" bubble_update "$ARG" | grep -c 'del equipo de su proyecto')" 1
curl -s -o /dev/null -X DELETE "$API/api/collections/bubbles/records/$TIB" -H "Authorization: $C"

ARG="{\"thread\":\"$GT\",\"sequence\":10}"
chk ">>> cada equipo ordena su secuencia: carol no toca la de gina" \
  "$(mcptext "$C" sequence_set "$ARG" | grep -c 'not found')" 1
ARG="{\"thread\":\"$GT\",\"sequence\":10}"
chk "...gina sí" "$(mcptext "$GI" sequence_set "$ARG" | j "['sequence']")" 10
ARG="{\"team\":\"$GP\"}"
chk ">>> y sequence_next de su equipo lo trae" \
  "$(mcptext "$GI" sequence_next "$ARG" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["team"]["id"]=="'"$GP"'", d["now"]["threads"][0]["id"]=="'"$GT"'")')" "True True"
ARG="{\"team\":\"$TI\"}"
chk "...pero no la línea de TI" \
  "$(mcptext "$GI" sequence_next "$ARG" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(any(t["id"]=="'"$GT"'" for s in (d.get("now"),d.get("then")) if s for t in s["threads"]))')" False
chk ">>> la cola de un equipo sólo trae sus proyectos" \
  "$(curl -s "$API/api/queue?kind=threads&state=open&team=$TI" -H "Authorization: $GI" | python3 -c 'import sys,json;print(all(c["workspace"]!="'"$GWS"'" for c in json.load(sys.stdin)["items"]))')" True

chk ">>> el canal de un equipo personal es sólo de su dueño" \
  "$(code "$API/api/channels?team=$GP" -H "Authorization: $C")" 403
chk "...gina sí lo ve" "$(code "$API/api/channels?team=$GP" -H "Authorization: $GI")" 200
chk ">>> un member no conecta el canal de su equipo" "$(code "$API/api/channels?team=$TI" -H "Authorization: $A")" 403

# Lo que la IA lee dice su contexto, y no asigna a quien no ve el trabajo.
ARG="{\"thread\":\"$GT\"}"
chk ">>> thread_get dice el equipo del hilo" \
  "$(mcptext "$GI" thread_get "$ARG" | python3 -c 'import sys,json;t=json.load(sys.stdin)["team"];print(t["name"], t["personal"])')" "Personal · Gina True"
chk ">>> project_list dice el equipo de cada proyecto, con nombre" \
  "$(mcptext "$GI" project_list '{}' | python3 -c 'import sys,json;print([p["team"]["name"] for p in json.load(sys.stdin) if p["id"]=="'"$GWS"'"][0])')" "Personal · Gina"
chk ">>> change_list dice el equipo de cada cambio" \
  "$(mcptext "$GI" change_list '{}' | python3 -c 'import sys,json;print([c["team"] for c in json.load(sys.stdin) if c.get("thread")=="'"$GT"'"][0])')" "Personal · Gina"
ARG="{\"project\":\"$GWS\"}"
chk ">>> people_list(project): sólo quienes ven el proyecto" \
  "$(mcptext "$GI" people_list "$ARG" | python3 -c 'import sys,json;ids={p["id"] for p in json.load(sys.stdin)};print("'"$GINA"'" in ids, "'"$BID"'" in ids, "'"$CID"'" in ids, "'"$AID"'" in ids)')" "True True False False"
ARG="{\"team\":\"$TI\"}"
chk ">>> people_list(team): quienes están en el equipo" \
  "$(mcptext "$C" people_list "$ARG" | python3 -c 'import sys,json;print("'"$AID"'" in {p["id"] for p in json.load(sys.stdin)})')" True
# Asignar es invitar: quien no ve el proyecto se suma a él, si quien asigna
# puede invitar (lead del proyecto o de su equipo).
ARG="{\"thread\":\"$GT\",\"assignees\":[\"$BID\"]}"
chk ">>> asignar a quien ya ve el proyecto (bob, invitado); la respuesta dice el equipo" \
  "$(mcptext "$GI" thread_update "$ARG" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["team"]["name"])')" "Personal · Gina"
ARG="{\"thread\":\"$GT\",\"assignees\":[\"$BID\",\"$AID\"]}"
chk ">>> un miembro (bob) no suma a nadie asignándolo" \
  "$(mcptext "$B" thread_update "$ARG" | grep -c 'pídele que lo invite')" 1
chk "...y alice sigue sin ver el proyecto" "$(code "$API/api/collections/workspaces/records/$GWS" -H "Authorization: $A")" 404
chk ">>> gina (lead) asigna a alice: queda invitada al proyecto" \
  "$(mcptext "$GI" thread_update "$ARG" | python3 -c 'import sys,json;print(len(json.load(sys.stdin).get("team",{})) > 0)')" True
chk "...y alice ya lo ve" "$(code "$API/api/collections/workspaces/records/$GWS" -H "Authorization: $A")" 200
chk "...pero no el equipo personal de gina" \
  "$(curl -s "$API/api/teams" -H "Authorization: $A" | python3 -c 'import sys,json;print(any(t["id"]=="'"$GP"'" for t in json.load(sys.stdin)["teams"]))')" False
ARG="{\"project\":\"$GWS\",\"name\":\"Módulo de dirección\"}"
GB=$(mcptext "$GI" bubble_create "$ARG")
GBID=$(echo "$GB" | j "['id']")
chk ">>> bubble_create dice el equipo del módulo" "$(echo "$GB" | j "['team']['name']")" "Personal · Gina"
ARG="{\"bubble\":\"$GBID\",\"owners\":[\"$CID\"]}"
chk ">>> hacer responsable de un módulo también invita (carol, el otro lead global)" \
  "$(mcptext "$GI" bubble_update "$ARG" | j "['team']['personal']")" True
chk "...carol ve ESE proyecto" "$(code "$API/api/collections/workspaces/records/$GWS" -H "Authorization: $C")" 200
chk "...y nada más del equipo personal" \
  "$(curl -s "$API/api/teams" -H "Authorization: $C" | python3 -c 'import sys,json;print(any(t["id"]=="'"$GP"'" for t in json.load(sys.stdin)["teams"]))')" False
# Por la API REST (la interfaz, también desde «Todos») vale lo mismo.
ARG="{\"owners\":[\"$GINA\",\"$HANK\"]}"
chk ">>> por REST, un miembro no invita asignando" \
  "$(code -X PATCH "$API/api/collections/bubbles/records/$GBID" -H "Authorization: $A" -H "$JS" -d "$ARG")" 400
chk "...y hank sigue sin verlo" "$(code "$API/api/collections/workspaces/records/$GWS" -H "Authorization: $HK")" 404
chk ">>> por REST, la lead del proyecto sí: hank queda invitado" \
  "$(code -X PATCH "$API/api/collections/bubbles/records/$GBID" -H "Authorization: $GI" -H "$JS" -d "$ARG")" 200
chk "...y lo ve" "$(code "$API/api/collections/workspaces/records/$GWS" -H "Authorization: $HK")" 200
OPS=$(curl -s -X POST "$API/api/teams" -H "Authorization: $C" -H "$JS" -d '{"name":"Operaciones"}' | j "['id']")
chk ">>> un lead global crea un equipo compartido" "$([ -n "$OPS" ] && echo si || echo no)" si
chk "...un member no" "$(code -X POST "$API/api/teams" -H "Authorization: $A" -H "$JS" -d '{"name":"Mío"}')" 403
chk "...y lo ve el otro lead global" \
  "$(curl -s "$API/api/teams" -H "Authorization: $GI" | python3 -c 'import sys,json;print(any(t["id"]=="'"$OPS"'" for t in json.load(sys.stdin)["teams"]))')" True
ARG="{\"user\":\"$AID\",\"role\":\"lead\"}"
chk ">>> carol pone a alice de lead de Operaciones" \
  "$(curl -s -X POST "$API/api/teams/$OPS/members" -H "Authorization: $C" -H "$JS" -d "$ARG" | python3 -c 'import sys,json;print([m["role"] for m in json.load(sys.stdin)["members"] if m["user"]=="'"$AID"'"][0])')" lead
ARG="{\"name\":\"Almacén\",\"team\":\"Operaciones\"}"
OWS=$(mcptext "$A" project_create "$ARG" | j "['id']")
chk "...y alice crea un proyecto ahí, por nombre del equipo" \
  "$(curl -s "$API/api/collections/workspaces/records/$OWS" -H "Authorization: $C" | j "['team']")" "$OPS"
chk ">>> alice no mueve el proyecto a TI: no es lead global" \
  "$(code -X PATCH "$API/api/collections/workspaces/records/$OWS" -H "Authorization: $A" -H "$JS" -d "{\"team\":\"$TI\"}")" 403
chk ">>> carol no lo mueve al equipo personal de gina" \
  "$(code -X PATCH "$API/api/collections/workspaces/records/$OWS" -H "Authorization: $C" -H "$JS" -d "{\"team\":\"$GP\"}")" 403
ARG="{\"team\":\"$TI\"}"
chk ">>> carol sí lo mueve a TI: lidera los dos" \
  "$(curl -s -X PATCH "$API/api/collections/workspaces/records/$OWS" -H "Authorization: $C" -H "$JS" -d "$ARG" | j "['team']")" "$TI"
chk ">>> un equipo con proyectos no se borra" \
  "$(code -X DELETE "$API/api/teams/$GP" -H "Authorization: $GI")" 400
chk ">>> uno vacío sí" "$(code -X DELETE "$API/api/teams/$OPS" -H "Authorization: $C")" 204
chk ">>> team_list: los equipos y el papel en cada uno" \
  "$(mcptext "$GI" team_list '{}' | python3 -c 'import sys,json;d=json.load(sys.stdin);print(sorted((t["name"],t["role"],t["personal"]) for t in d["teams"]))')" "[('Personal · Gina', 'lead', True), ('TI', 'lead', False)]"

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
