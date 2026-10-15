// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	api "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database/types"
)

// CreateWorker creates a new worker in the database.
//
// When the worker carries a generation identifier (registration flow),
// creation and consumption of the authorizing register token happen in
// a single transaction. The register token is only considered consumed
// when it is still active, unexpired, and scoped to the worker
// hostname; otherwise the worker insert is rolled back and the
// generation is left unestablished. Workers created with the shared
// symmetric server-worker token carry no generation and skip the
// consumption check.
func (e *Engine) CreateWorker(ctx context.Context, w *api.Worker) (*api.Worker, error) {
	e.logger.WithFields(logrus.Fields{
		"worker": w.GetHostname(),
	}).Tracef("creating worker %s", w.GetHostname())

	// cast the API type to database type
	worker := types.WorkerFromAPI(w)

	// validate the necessary fields are populated
	err := worker.Validate()
	if err != nil {
		return nil, err
	}

	// no generation means the shared symmetric token mode:
	// persist the worker without consuming a register token
	if len(w.GetGeneration()) == 0 {
		// send query to the database
		result := e.client.
			WithContext(ctx).
			Table(constants.TableWorker).
			Create(worker)

		return worker.ToAPI(w.GetRunningBuilds()), result.Error
	}

	// registration flow: establish the worker generation and mark the
	// authorizing register token used atomically
	err = e.client.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// insert the worker row for the new generation
		if err := tx.
			Table(constants.TableWorker).
			Create(worker).Error; err != nil {
			return err
		}

		// conditionally consume the register token. This guards against
		// replayed, already-used, expired, or wrong-hostname tokens, and
		// is race-safe with concurrent registration attempts.
		result := tx.
			Table(constants.TableWorkerRegisterToken).
			Where("token_id = ?", w.GetGeneration()).
			Where("hostname = ?", w.GetHostname()).
			Where("status = ?", constants.WorkerRegisterTokenStatusActive).
			Where("expires_at > ?", time.Now().Unix()).
			Updates(map[string]any{
				"status":    constants.WorkerRegisterTokenStatusUsed,
				"worker_id": worker.ID.Int64,
			})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected != 1 {
			// roll back the worker insert so the hostname is never left
			// occupied by a generation without valid credentials
			return ErrInvalidWorkerRegisterToken
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return worker.ToAPI(w.GetRunningBuilds()), nil
}
