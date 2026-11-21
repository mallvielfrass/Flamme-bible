package server

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// minPostgresMajor — минимальный мажор PostgreSQL.
// Схема использует обычные возможности 16+ (jsonb, uuid, timestamptz) и не привязана к патчу 16.3.
const minPostgresMajor = 16

func Migrate(ctx context.Context, db *pgxpool.Pool) error {
	if err := checkPostgres(ctx, db); err != nil {
		return err
	}
	for stmt := range strings.SplitSeq(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("миграция: %w", err)
		}
	}
	if err := migrateMVP(ctx, db); err != nil {
		return err
	}
	if err := migrateSemantic(ctx, db); err != nil {
		return err
	}
	if err := migratePlansUI(ctx, db); err != nil {
		return err
	}
	if err := migrateAdmin(ctx, db); err != nil {
		return err
	}
	return migrateEmbedQueue(ctx, db)
}

func checkPostgres(ctx context.Context, db *pgxpool.Pool) error {
	major, version, err := postgresMajor(ctx, db)
	if err != nil {
		return fmt.Errorf("версия PostgreSQL: %w", err)
	}
	if major < minPostgresMajor {
		return fmt.Errorf("нужен PostgreSQL %d или новее, сейчас %s", minPostgresMajor, version)
	}
	return nil
}

func postgresMajor(ctx context.Context, db *pgxpool.Pool) (int, string, error) {
	var num int
	var version string
	err := db.QueryRow(ctx, `SELECT current_setting('server_version_num')::int, current_setting('server_version')`).Scan(&num, &version)
	if err != nil {
		return 0, "", err
	}
	if num <= 0 {
		return 0, version, fmt.Errorf("не разобрана версия PostgreSQL %q", version)
	}
	return majorFromVersionNum(num), version, nil
}

func majorFromVersionNum(num int) int {
	return num / 10000
}
