package pgutil

import (
	"context"
	"testing"
)

// Migrate y MigrateDown son no-ops en este entorno (las tablas las crea el
// init script de Postgres) — ambas ignoran el pool, por lo que es seguro
// pasar nil sin necesidad de una conexión real.

func TestMigrate_IsNoOp(t *testing.T) {
	if err := Migrate(context.Background(), nil, "migrations/authorization"); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
}

func TestMigrateDown_IsNoOp(t *testing.T) {
	if err := MigrateDown(nil, "migrations/authorization", 1); err != nil {
		t.Fatalf("MigrateDown() error = %v, want nil", err)
	}
}
