// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"

	"github.com/go-vela/server/constants"
)

const (
	// CreatePostgresTable represents a query to create the Postgres workers table.
	CreatePostgresTable = `
CREATE TABLE
IF NOT EXISTS
workers (
	id                     BIGSERIAL PRIMARY KEY,
	hostname               VARCHAR(250),
	address                VARCHAR(250),
	routes                 VARCHAR(1000),
	active                 BOOLEAN,
	status                 VARCHAR(50),
	last_status_update_at  BIGINT,
	running_build_ids      VARCHAR(500),
	last_build_started_at  BIGINT,
	last_build_finished_at BIGINT,
	last_checked_in        BIGINT,
	build_limit            INTEGER,
	generation             VARCHAR(36),
	UNIQUE(hostname)
);
`
	// CreateSqliteTable represents a query to create the Sqlite workers table.
	CreateSqliteTable = `
CREATE TABLE
IF NOT EXISTS
workers (
	id                     INTEGER PRIMARY KEY AUTOINCREMENT,
	hostname               TEXT,
	address                TEXT,
	routes                 TEXT,
	active                 BOOLEAN,
	status                 VARCHAR(50),
	last_status_update_at  INTEGER,
	running_build_ids      VARCHAR(500),
	last_build_started_at  INTEGER,
	last_build_finished_at INTEGER,
	last_checked_in	       INTEGER,
	build_limit            INTEGER,
	generation             TEXT,
	UNIQUE(hostname)
);
`

	// CreatePostgresRegisterTokenTable represents a query to create the
	// Postgres worker register tokens table. Each row is the server-side,
	// single-use authorization for one worker registration (one worker
	// lifecycle generation).
	CreatePostgresRegisterTokenTable = `
CREATE TABLE
IF NOT EXISTS
worker_register_tokens (
	id         BIGSERIAL PRIMARY KEY,
	token_id   VARCHAR(36) UNIQUE NOT NULL,
	hostname   VARCHAR(250) NOT NULL,
	status     VARCHAR(50) NOT NULL,
	worker_id  BIGINT,
	expires_at BIGINT NOT NULL,
	created_at BIGINT NOT NULL
);
`
	// CreateSqliteRegisterTokenTable represents a query to create the
	// Sqlite worker register tokens table.
	CreateSqliteRegisterTokenTable = `
CREATE TABLE
IF NOT EXISTS
worker_register_tokens (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	token_id   TEXT UNIQUE NOT NULL,
	hostname   TEXT NOT NULL,
	status     TEXT NOT NULL,
	worker_id  INTEGER,
	expires_at INTEGER NOT NULL,
	created_at INTEGER NOT NULL
);
`
)

// CreateWorkerTable creates the workers table in the database.
func (e *Engine) CreateWorkerTable(ctx context.Context, driver string) error {
	e.logger.Tracef("creating workers table")

	// handle the driver provided to create the table
	switch driver {
	case constants.DriverPostgres:
		// create the workers table for Postgres
		return e.client.
			WithContext(ctx).
			Exec(CreatePostgresTable).Error
	case constants.DriverSqlite:
		fallthrough
	default:
		// create the workers table for Sqlite
		return e.client.
			WithContext(ctx).
			Exec(CreateSqliteTable).Error
	}
}

// CreateWorkerRegisterTokenTable creates the worker register tokens
// table in the database.
func (e *Engine) CreateWorkerRegisterTokenTable(ctx context.Context, driver string) error {
	e.logger.Tracef("creating worker register tokens table")

	// handle the driver provided to create the table
	switch driver {
	case constants.DriverPostgres:
		// create the worker register tokens table for Postgres
		return e.client.
			WithContext(ctx).
			Exec(CreatePostgresRegisterTokenTable).Error
	case constants.DriverSqlite:
		fallthrough
	default:
		// create the worker register tokens table for Sqlite
		return e.client.
			WithContext(ctx).
			Exec(CreateSqliteRegisterTokenTable).Error
	}
}
