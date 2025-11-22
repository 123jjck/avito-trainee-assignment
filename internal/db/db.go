package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(time.Hour)
	return db, nil
}

func RunMigrations(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS teams (
			team_name TEXT PRIMARY KEY
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			user_id TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			team_name TEXT NOT NULL REFERENCES teams(team_name),
			is_active BOOLEAN NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS pull_requests (
			pull_request_id TEXT PRIMARY KEY,
			pull_request_name TEXT NOT NULL,
			author_id TEXT NOT NULL REFERENCES users(user_id),
			status TEXT NOT NULL CHECK (status IN ('OPEN', 'MERGED')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			merged_at TIMESTAMPTZ NULL
		);`,
		`CREATE TABLE IF NOT EXISTS pr_reviewers (
			pull_request_id TEXT NOT NULL REFERENCES pull_requests(pull_request_id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(user_id),
			PRIMARY KEY (pull_request_id, user_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_users_team ON users(team_name);`,
		`CREATE INDEX IF NOT EXISTS idx_pr_reviewers_user ON pr_reviewers(user_id);`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply migration: %w", err)
		}
	}
	return nil
}
