// SPDX-License-Identifier: Apache-2.0

package worker

import "context"

const (
	// CreateHostnameAddressIndex represents a query to create an
	// index on the workers table for the hostname and address columns.
	CreateHostnameAddressIndex = `
CREATE INDEX
IF NOT EXISTS
workers_hostname_address
ON workers (hostname, address);
`

	// CreateRegisterTokenHostnameIndex represents a query to create an
	// index on the worker register tokens table for the hostname column,
	// allowing every token issued for a hostname to be revoked atomically
	// when the worker is deleted.
	CreateRegisterTokenHostnameIndex = `
CREATE INDEX
IF NOT EXISTS
worker_register_tokens_hostname
ON worker_register_tokens (hostname);
`

	// AlterPostgresWorkerGeneration represents an idempotent query to add
	// the generation column to a workers table created before generations
	// existed.
	AlterPostgresWorkerGeneration = `
ALTER TABLE workers
ADD COLUMN IF NOT EXISTS generation VARCHAR(36);
`

	// AlterSqliteWorkerGeneration represents a query to add the
	// generation column to a Sqlite workers table created before
	// generations existed. Sqlite has no "ADD COLUMN IF NOT EXISTS";
	// a duplicate-column error from this statement is ignored.
	AlterSqliteWorkerGeneration = `
ALTER TABLE workers
ADD COLUMN generation TEXT;
`
)

// CreateWorkerIndexes creates the indexes for the workers table in the database.
func (e *Engine) CreateWorkerIndexes(ctx context.Context) error {
	e.logger.Tracef("creating indexes for workers table")

	// create the hostname and address columns index for the workers table
	return e.client.
		WithContext(ctx).
		Exec(CreateHostnameAddressIndex).Error
}

// CreateWorkerRegisterTokenIndexes creates the indexes for the worker
// register tokens table in the database.
func (e *Engine) CreateWorkerRegisterTokenIndexes(ctx context.Context) error {
	e.logger.Tracef("creating indexes for worker register tokens table")

	// create the hostname index for the worker register tokens table
	return e.client.
		WithContext(ctx).
		Exec(CreateRegisterTokenHostnameIndex).Error
}
