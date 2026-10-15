// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database"
	dbworker "github.com/go-vela/server/database/worker"
	"github.com/go-vela/server/router/middleware/claims"
	"github.com/go-vela/server/router/middleware/worker"
	"github.com/go-vela/server/util"
)

// swagger:operation PUT /api/v1/workers/{worker} workers UpdateWorker
//
// Update a worker
//
// ---
// produces:
// - application/json
// parameters:
// - in: body
//   name: body
//   description: The worker object with the fields to be updated
//   required: true
//   schema:
//     "$ref": "#/definitions/Worker"
// - in: path
//   name: worker
//   description: Name of the worker
//   required: true
//   type: string
// security:
//   - ApiKeyAuth: []
// responses:
//   '200':
//     description: Successfully updated the worker
//     schema:
//       "$ref": "#/definitions/Worker"
//   '400':
//     description: Invalid request payload or path
//     schema:
//       "$ref": "#/definitions/Error"
//   '401':
//     description: Unauthorized
//     schema:
//       "$ref": "#/definitions/Error"
//   '404':
//     description: Not found
//     schema:
//       "$ref": "#/definitions/Error"
//   '500':
//     description: Unexpected server error
//     schema:
//       "$ref": "#/definitions/Error"

// UpdateWorker represents the API handler to
// update a worker.
func UpdateWorker(c *gin.Context) {
	// capture middleware values
	l := c.MustGet("logger").(*logrus.Entry)
	cl := claims.Retrieve(c)
	w := worker.Retrieve(c)
	ctx := c.Request.Context()

	// an auth token may only update the worker generation it was minted
	// for. this stops auth tokens from a deleted generation from
	// updating a newer same-hostname worker. platform admin users and the
	// explicitly configured shared symmetric server-worker token bypass
	// generation enforcement.
	if !cl.IsAdmin &&
		cl.TokenType != constants.ServerWorkerTokenType &&
		cl.Generation != w.GetGeneration() {
		l.WithFields(logrus.Fields{
			"claims_generation": cl.Generation,
			"worker_generation": w.GetGeneration(),
		}).Warnf("rejected update for worker %s from a different generation", w.GetHostname())

		retErr := fmt.Errorf("unable to update worker %s: auth token is not valid for the current worker generation", w.GetHostname())

		util.HandleError(c, http.StatusUnauthorized, retErr)

		return
	}

	l.Debugf("updating worker %s", w.GetHostname())

	// capture body from API request
	input := new(types.Worker)

	err := c.Bind(input)
	if err != nil {
		retErr := fmt.Errorf("unable to decode JSON for worker %s: %w", w.GetHostname(), err)

		util.HandleError(c, http.StatusBadRequest, retErr)

		return
	}

	if len(input.GetAddress()) > 0 {
		// update address if set
		w.SetAddress(input.GetAddress())
	}

	if len(input.GetRoutes()) > 0 {
		// update routes if set
		w.SetRoutes(input.GetRoutes())
	}

	if input.Active != nil {
		// update active if set
		w.SetActive(input.GetActive())
	}

	if input.RunningBuilds != nil {
		// update runningBuildIDs if set
		w.SetRunningBuilds(input.GetRunningBuilds())
	}

	if len(input.GetStatus()) > 0 {
		// update status if set
		w.SetStatus(input.GetStatus())
	}

	if input.GetLastStatusUpdateAt() > 0 {
		// update lastStatusUpdateAt if set
		w.SetLastStatusUpdateAt(input.GetLastStatusUpdateAt())
	}

	if input.GetLastBuildStartedAt() > 0 {
		// update lastBuildStartedAt if set
		w.SetLastBuildStartedAt(input.GetLastBuildStartedAt())
	}

	if input.GetLastBuildFinishedAt() > 0 {
		// update lastBuildFinishedAt if set
		w.SetLastBuildFinishedAt(input.GetLastBuildFinishedAt())
	}

	// send API call to update the worker
	w, err = database.FromContext(c).UpdateWorker(ctx, w)
	if err != nil {
		// the worker was deleted and recreated as a newer generation
		// between loading the record and writing it; the stale write was
		// conditionally rejected
		if errors.Is(err, dbworker.ErrStaleWorker) {
			retErr := fmt.Errorf("unable to update worker %s: %w", w.GetHostname(), err)

			util.HandleError(c, http.StatusConflict, retErr)

			return
		}

		retErr := fmt.Errorf("unable to update worker %s: %w", w.GetHostname(), err)

		util.HandleError(c, http.StatusInternalServerError, retErr)

		return
	}

	c.JSON(http.StatusOK, w)
}
