package server

import (
	"context"
	"fmt"

	"tokenhub/backend/internal/dbschema"
)

// providerTenancyMigration adds the providers.owner_team_id column that scopes
// a provider channel to a team. Empty values keep the platform-owned semantics
// that existing rows already have, so the expansion is backward compatible on
// both dialects.
func providerTenancyMigration() dbschema.Migration {
	return dbschema.Migration{
		Version:          7,
		Name:             "add-provider-owner-team",
		Go:               addProviderOwnerTeamColumn,
		ChecksumOverride: "tokenhub-schema-provider-owner-team-v1",
		StatementBudget:  10,
	}
}

func addProviderOwnerTeamColumn(ctx context.Context, db dbschema.MigrationExecer) error {
	// current_schema() is available on PostgreSQL. SQLite reports an ordinary
	// statement error without aborting its transaction, so this is a safe,
	// read-only dialect probe inside the migration callback (same approach as
	// the audit correlation migration).
	var schema string
	if err := db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema); err == nil {
		if _, err := db.ExecContext(ctx, `ALTER TABLE providers ADD COLUMN IF NOT EXISTS owner_team_id text`); err != nil {
			return fmt.Errorf("add PostgreSQL provider owner column: %w", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_providers_owner_team_id ON providers (owner_team_id)`); err != nil {
			return fmt.Errorf("index PostgreSQL provider owner column: %w", err)
		}
		return nil
	}

	exists, err := sqliteColumnExists(ctx, db, "providers", "owner_team_id")
	if err != nil {
		return fmt.Errorf("inspect SQLite provider owner column: %w", err)
	}
	if !exists {
		if _, err := db.ExecContext(ctx, `ALTER TABLE providers ADD COLUMN owner_team_id text`); err != nil {
			return fmt.Errorf("add SQLite provider owner column: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_providers_owner_team_id ON providers (owner_team_id)`); err != nil {
		return fmt.Errorf("index SQLite provider owner column: %w", err)
	}
	return nil
}
