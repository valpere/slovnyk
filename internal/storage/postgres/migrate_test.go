package postgres

import (
	"context"
	"errors"
	"testing"
)

func TestConnectRejectsConfigWithoutLeakingValues(t *testing.T) {
	for _, dsn := range []string{"", "postgres://user:synthetic-secret@host:invalid/db"} {
		_, err := Connect(context.Background(), dsn)
		if !errors.Is(err, ErrConfig) || err.Error() != "invalid_database_configuration" {
			t.Fatalf("unsafe config failure")
		}
	}
}
