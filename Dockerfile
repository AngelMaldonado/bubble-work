# La imagen: un binario, git, y nada más.
#
# El bundle de la interfaz está commiteado y embebido, así que construir no
# necesita bun — sólo Go. Es la misma propiedad que hace que `just build`
# funcione en un clon recién hecho, aprovechada aquí.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src

# Las dependencias primero, en su propia capa: el código cambia en cada commit y
# `go.mod` casi nunca, así que la descarga se reutiliza.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# La versión la pasa quien construye (`--build-arg VERSION=$(git describe …)`).
# El repositorio no viaja dentro de la imagen, así que el binario no puede
# averiguarla por su cuenta — y un binario que no sabe qué es no puede decirlo
# cuando alguien pregunte por qué se comporta raro.
ARG VERSION=dev
# Compila CRUZADO en vez de emular: la etapa corre en la arquitectura de quien
# construye y Go produce el binario de la que se pide. Es la diferencia entre
# sacar linux/amd64 desde una máquina arm64 en segundos o bajo qemu en minutos.
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-X main.version=${VERSION}" -o /out/bubble .

FROM alpine:3.21
# git de verdad, no una biblioteca: el árbol de markdown de cada workspace es un
# repositorio que una persona tiene que poder clonar y leer con sus propias
# herramientas, y el servidor lo escribe llamando al mismo programa que ellas.
# ca-certificates para hablar con fuera; tzdata porque las fechas de renovación
# del inventario se leen en la zona de quien mira.
RUN apk add --no-cache git ca-certificates tzdata

# Los DOS directorios que son estado. Declarados aquí para que un `docker run`
# sin compose tampoco pierda nada por descuido.
ENV BUBBLE_DATA=/data/pb_data \
    BUBBLE_REPOS=/data/repos \
    BUBBLE_HTTP=0.0.0.0:8090
VOLUME ["/data"]

COPY --from=build /out/bubble /usr/local/bin/bubble
EXPOSE 8090

# `serve` corre las migraciones pendientes al arrancar. Es la puerta de un solo
# sentido de toda actualización, y por eso `scripts/update.sh` ofrece hacer una
# copia justo antes.
ENTRYPOINT ["/usr/local/bin/bubble"]
CMD ["serve", "--http", "0.0.0.0:8090", "--dir", "/data/pb_data"]
