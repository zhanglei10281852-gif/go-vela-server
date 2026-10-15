// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"

	"github.com/sirupsen/logrus"

	api "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database/types"
)

// UpdateWorker updates an existing worker in the database.
//
// The write is conditional on the worker's current lifecycle
// generation: a generation-bound worker is only updated when the
// caller's record still carries the same generation recorded in the
// database. If the worker was deleted and recreated (a newer
// generation) after the caller loaded its record, the update matches
// no rows and ErrStaleWorker is returned, preventing late requests
// from an older generation from overwriting the new worker's address,
// routes, running builds, status, or check-in time.
func (e *Engine) UpdateWorker(ctx context.Context, w *api.Worker) (*api.Worker, error) {
	e.logger.WithFields(logrus.Fields{
		"worker": w.GetHostname(),
	}).Tracef("updating worker %s", w.GetHostname())

	// cast the API type to database type
	worker := types.WorkerFromAPI(w)

	// validate the necessary fields are populated
	err := worker.Validate()
	if err != nil {
		return nil, err
	}

	// build the conditional write. the statement is an explicit UPDATE
	// (never an upsert). gorm restricts struct updates to the record's
	// primary key; bound generations additionally require the stored
	// generation to match, while legacy and symmetric workers (no
	// generation) match on ID only.
	query := e.client.
		WithContext(ctx).
		Table(constants.TableWorker)

	if len(w.GetGeneration()) > 0 {
		query = query.Where("generation = ?", w.GetGeneration())
	}

	// send query to the database. Select("*") ensures every column
	// (including zero-valued ones) is written, matching prior Save
	// behavior, while never inserting a missing or stale row.
	result := query.Select("*").Updates(worker)
	if result.Error != nil {
		return nil, result.Error
	}

	// no row matched: the record belongs to a deleted or superseded
	// generation and the write must not land on a newer same-hostname row
	if result.RowsAffected == 0 {
		return nil, ErrStaleWorker
	}

	return worker.ToAPI(w.GetRunningBuilds()), nil
}
