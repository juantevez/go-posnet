#!/usr/bin/env bash
# =============================================================================
# gen-proto.sh — Regenera el código Go desde los archivos .proto
#
# Prerequisitos (instalar una sola vez):
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#
#   En Ubuntu/Debian:
#     apt install protobuf-compiler
#
#   En macOS:
#     brew install protobuf
#
# Uso:
#   ./scripts/gen-proto.sh              # Regenera todos los protos
#   ./scripts/gen-proto.sh authorization # Regenera solo uno
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_ROOT="$REPO_ROOT/posnet/pkg/proto"

# Verificar que protoc esté instalado
if ! command -v protoc &> /dev/null; then
  echo "ERROR: protoc no encontrado."
  echo "  Ubuntu:  apt install protobuf-compiler"
  echo "  macOS:   brew install protobuf"
  exit 1
fi

# Verificar que los plugins de Go estén instalados
if ! command -v protoc-gen-go &> /dev/null; then
  echo "ERROR: protoc-gen-go no encontrado."
  echo "  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
  exit 1
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
  echo "ERROR: protoc-gen-go-grpc no encontrado."
  echo "  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
  exit 1
fi

# Agregar $GOPATH/bin al PATH para que protoc encuentre los plugins
export PATH="$PATH:$(go env GOPATH)/bin"

# Detectar el directorio de includes de protobuf (google/protobuf/timestamp.proto etc.)
# En Ubuntu/Debian: /usr/include
# En macOS con brew: $(brew --prefix protobuf)/include
detect_include_dir() {
  for candidate in \
    "/usr/include" \
    "/usr/local/include" \
    "$(brew --prefix protobuf 2>/dev/null)/include" \
    "$(go env GOPATH)/pkg/mod/github.com/protocolbuffers/protobuf@*/src"
  do
    # Expandir glob si aplica
    for expanded in $candidate; do
      if [ -f "$expanded/google/protobuf/timestamp.proto" ]; then
        echo "$expanded"
        return
      fi
    done
  done
  echo ""
}

INCLUDE_DIR="$(detect_include_dir)"
if [ -z "$INCLUDE_DIR" ]; then
  echo "ERROR: no se encontró google/protobuf/timestamp.proto."
  echo "  Ubuntu:  sudo apt install protobuf-compiler libprotobuf-dev"
  echo "  macOS:   brew install protobuf"
  exit 1
fi

# ─── Servicios a generar ──────────────────────────────────────────────────────
# Formato: "directorio_relativo_en_proto/|archivo.proto"
ALL_PROTOS=(
  "terminalgateway/v1|terminal_gateway.proto"
  "authorization/v1|authorization.proto"
  "notification/v1|notification.proto"
)

# Filtrar por argumento si se pasó uno
FILTER="${1:-}"
if [ -n "$FILTER" ]; then
  FILTERED=()
  for entry in "${ALL_PROTOS[@]}"; do
    if [[ "$entry" == *"$FILTER"* ]]; then
      FILTERED+=("$entry")
    fi
  done
  if [ ${#FILTERED[@]} -eq 0 ]; then
    echo "ERROR: no se encontró ningún proto con el filtro '$FILTER'"
    echo "Disponibles: terminalgateway, authorization, notification"
    exit 1
  fi
  ALL_PROTOS=("${FILTERED[@]}")
fi

# ─── Generación ───────────────────────────────────────────────────────────────
echo "============================================="
echo "  gen-proto.sh — Generando código Go"
echo "  Proto root: $PROTO_ROOT"
echo "============================================="
echo ""

for entry in "${ALL_PROTOS[@]}"; do
  DIR="${entry%%|*}"
  FILE="${entry##*|}"
  PROTO_DIR="$PROTO_ROOT/$DIR"
  PROTO_FILE="$PROTO_DIR/$FILE"

  if [ ! -f "$PROTO_FILE" ]; then
    echo "⚠  Archivo no encontrado: $PROTO_FILE — saltando"
    continue
  fi

  echo "▶ Generando: $DIR/$FILE"

  # --proto_path apunta al directorio del .proto concreto.
  # Esto permite que protoc resuelva el archivo como nombre simple ("terminal_gateway.proto")
  # y que paths=source_relative deposite el .pb.go en ese mismo directorio.
  #
  # google/protobuf/timestamp.proto es resuelto por protoc desde su instalación
  # del sistema (/usr/include o /usr/local/include) — no hay que pasarlo a mano.
  protoc \
    --proto_path="$PROTO_DIR" \
    --proto_path="$INCLUDE_DIR" \
    --go_out="$PROTO_DIR" \
    --go_opt=paths=source_relative \
    --go-grpc_out="$PROTO_DIR" \
    --go-grpc_opt=paths=source_relative \
    "$FILE"

  echo "  ✔ pkg/proto/$DIR/$(basename "$FILE" .proto).pb.go"
  echo "  ✔ pkg/proto/$DIR/$(basename "$FILE" .proto)_grpc.pb.go"
done

echo ""
echo "============================================="
echo "  Generación completada"
echo "============================================="
echo ""
echo "  Recordatorio:"
echo "  - Los archivos .pb.go se COMMITEAN al repositorio"
echo "  - Nunca editar .pb.go a mano — son generados"
echo "  - Cambios en .proto requieren correr este script y"
echo "    commitear los .pb.go actualizados"
echo ""
