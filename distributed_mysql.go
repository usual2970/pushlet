package pushlet

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/usual2970/novaque"
	"github.com/usual2970/novaque/driver/mysql"
)

// MySQLPoolDefaults are recommended pool settings for embedders opening a
// dedicated *sql.DB for pushlet distributed mode.
type MySQLPoolDefaults struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	PingTimeout     time.Duration
}

// DefaultMySQLPoolDefaults matches common production embedder settings.
func DefaultMySQLPoolDefaults() MySQLPoolDefaults {
	return MySQLPoolDefaults{
		MaxOpenConns:    16,
		MaxIdleConns:    4,
		ConnMaxLifetime: 30 * time.Minute,
		PingTimeout:     5 * time.Second,
	}
}

// OpenMySQLNovaque opens a MySQL pool and novaque client for distributed mode.
// The caller owns db and must Close db after Pushlet.Stop.
func OpenMySQLNovaque(ctx context.Context, dsn string, novaqueOpts novaque.Options, pool MySQLPoolDefaults) (*sql.DB, *novaque.Client, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, nil, fmt.Errorf("pushlet: empty MySQL DSN")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("pushlet: mysql open: %w", err)
	}
	if pool.MaxOpenConns > 0 {
		db.SetMaxOpenConns(pool.MaxOpenConns)
	}
	if pool.MaxIdleConns > 0 {
		db.SetMaxIdleConns(pool.MaxIdleConns)
	}
	if pool.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(pool.ConnMaxLifetime)
	}
	pingTimeout := pool.PingTimeout
	if pingTimeout <= 0 {
		pingTimeout = 5 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("pushlet: mysql ping: %w", err)
	}
	client, err := novaque.Open(mysql.New(db), novaqueOpts)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("pushlet: novaque open: %w", err)
	}
	return db, client, nil
}
