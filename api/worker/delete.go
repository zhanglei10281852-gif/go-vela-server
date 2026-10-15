// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/go-vela/server/database"
	dbworker "github.com/go-vela/server/database/worker"
	"github.com/go-vela/server/router/middleware/worker"
	"github.com/go-vela/server/util"
)

// swagger:operation DELETE /api/v1/workers/{worker} workers DeleteWorker
//
// Delete a worker
//
// ---
// produces:
// - application/json
// parameters:
// - in: path
//   name: worker
//   description: Name of the worker
//   required: true
//   type: string
// security:
//   - ApiKeyAuth: []
// responses:
//   '200':
//     description: Successfully deleted worker
//     schema:
//       type: string
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

// DeleteWorker represents the API handler to remove a worker.
func DeleteWorker(c *gin.Context) {
	// capture middleware values
	l := c.MustGet("logger").(*logrus.Entry)
	w := worker.Retrieve(c)
	ctx := c.Request.Context()

	l.Debugf("deleting worker %s", w.GetHostname())

	// send API call to remove the worker and revoke all of its
	// register tokens
	err := database.FromContext(c).DeleteWorker(ctx, w)
	if err != nil {
		// the worker was already removed by a concurrent request
		if errors.Is(err, dbworker.ErrStaleWorker) {
			retErr := fmt.Errorf("unable to delete worker %s: %w", w.GetHostname(), err)

			util.HandleError(c, http.StatusNotFound, retErr)

			return
		}

		retErr := fmt.Errorf("unable to delete worker %s: %w", w.GetHostname(), err)

		util.HandleError(c, http.StatusInternalServerError, retErr)

		return
	}

	c.JSON(http.StatusOK, fmt.Sprintf("worker %s deleted", w.GetHostname()))
}
