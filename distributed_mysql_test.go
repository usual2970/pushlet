package pushlet

import (
	"context"
	"testing"
)

func TestOpenMySQLNovaqueEmptyDSN(t *testing.T) {
	_, _, err := OpenMySQLNovaque(context.Background(), "  ", DefaultDistributedOptions().Novaque, DefaultMySQLPoolDefaults())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenMySQLNovaqueInvalidDSN(t *testing.T) {
	_, _, err := OpenMySQLNovaque(context.Background(), "not-a-dsn", DefaultDistributedOptions().Novaque, DefaultMySQLPoolDefaults())
	if err == nil {
		t.Fatal("expected error")
	}
}
