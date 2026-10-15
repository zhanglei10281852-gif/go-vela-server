// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	api "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database/types"
)

// DeleteWorker deletes an existing worker from the database.
//
// The worker row and every register token issued for its hostname are
// removed in a single transaction. Once the delete commits, register
// tokens from any prior generation cannot establish a same-named
// worker and auth tokens from prior generations can no longer match
// the record, so neither can affect a worker later registered with the
// same hostname.
func (e *Engine) DeleteWorker(ctx context.Context, w *api.Worker) error {
	e.logger.WithFields(logrus.Fields{
		"worker": w.GetHostname(),
	}).Tracef("deleting worker %s", w.GetHostname())

	// cast the API type to database type
	worker := types.WorkerFromAPI(w)

	return e.client.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// delete the worker row; a missing row means it was already
		// removed by a concurrent delete
		result := tx.
			Table(constants.TableWorker).
			Delete(worker)
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return ErrStaleWorker
		}

		// revoke every register token (pending or consumed) issued for
		// this hostname, preventing old generations from being
		// re-established after the delete
		return tx.
			Table(constants.TableWorkerRegisterToken).
			Where("hostname = ?", w.GetHostname()).
			Delete(new(types.WorkerRegisterToken)).
			Error
	})
}
