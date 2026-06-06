#!/usr/bin/env bash
# =============================================================================
# gen-proto.sh — Regenera el código Go desde los archivos .proto
#
# Los .proto NO usan google/protobuf/timestamp.proto — usan string RFC3339.
# Esto elimina la dependencia del include dir del sistema.
#
# Prerequisitos:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#   apt install protobuf-compiler  (Ubuntu)
#   brew install protobuf          (macOS)
#
# Uso:
#   ./scripts/gen-proto.sh              # Todos los protos
#   ./scripts/gen-proto.sh authorization # Solo uno
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_ROOT="$REPO_ROOT/pkg/proto"

# Verificar herramientas
if ! command -v protoc &> /dev/null; then
  echo "ERROR: protoc no encontrado."
  echo "  Ubuntu: sudo apt install protobuf-compiler"
  echo "  macOS:  brew install protobuf"
  exit 1
fi

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

export PATH="$PATH:$(go env GOPATH)/bin"

# Servicios a generar
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
  # paths=source_relative deposita el .pb.go en ese mismo directorio.
  # Sin --proto_path al include dir del sistema porque no usamos well-known types.
  protoc \
    --proto_path="$PROTO_DIR" \
    --go_out="$PROTO_DIR" \
    --go_opt=paths=source_relative \
    --go-grpc_out="$PROTO_DIR" \
    --go-grpc_opt=paths=source_relative \
    "$FILE"

  echo "  ✔ $DIR/$(basename "$FILE" .proto).pb.go"
  echo "  ✔ $DIR/$(basename "$FILE" .proto)_grpc.pb.go"
  echo ""
done

echo "============================================="
echo "  Generación completada"
echo "============================================="
echo ""
echo "  IMPORTANTE: commitear los .pb.go generados."
echo "  Nunca editar .pb.go a mano."
echo ""
