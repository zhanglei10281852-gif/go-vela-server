// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database"
	dbworker "github.com/go-vela/server/database/worker"
	"github.com/go-vela/server/internal/token"
	"github.com/go-vela/server/router/middleware/claims"
	"github.com/go-vela/server/util"
)

// swagger:operation POST /api/v1/workers workers CreateWorker
//
// Create a worker
//
// ---
// produces:
// - application/json
// parameters:
// - in: body
//   name: body
//   description: Worker object to create
//   required: true
//   schema:
//     "$ref": "#/definitions/Worker"
// security:
//   - ApiKeyAuth: []
// responses:
//   '201':
//     description: Successfully created the worker and retrieved auth token
//     schema:
//       "$ref": "#/definitions/Token"
//   '400':
//     description: Invalid request payload
//     schema:
//       "$ref": "#/definitions/Error"
//   '401':
//     description: Unauthorized
//     schema:
//       "$ref": "#/definitions/Error"
//   '409':
//     description: Worker with the hostname already exists
//     schema:
//       "$ref": "#/definitions/Error"
//   '500':
//     description: Unexpected server error
//     schema:
//       "$ref": "#/definitions/Error"

// CreateWorker represents the API handler to
// create a worker.
func CreateWorker(c *gin.Context) {
	// capture middleware values
	l := c.MustGet("logger").(*logrus.Entry)
	cl := claims.Retrieve(c)
	ctx := c.Request.Context()

	// capture body from API request
	input := new(types.Worker)

	err := c.Bind(input)
	if err != nil {
		retErr := fmt.Errorf("unable to decode JSON for new worker: %w", err)

		util.HandleError(c, http.StatusBadRequest, retErr)

		return
	}

	// verify input host name matches worker hostname
	if !strings.EqualFold(cl.TokenType, constants.ServerWorkerTokenType) && !strings.EqualFold(cl.Subject, input.GetHostname()) {
		retErr := fmt.Errorf("unable to add worker; claims subject %s does not match worker hostname %s", cl.Subject, input.GetHostname())

		util.HandleError(c, http.StatusBadRequest, retErr)

		return
	}

	input.SetLastCheckedIn(time.Now().Unix())

	l.Debugf("creating new worker %s", input.GetHostname())

	switch cl.TokenType {
	// if symmetric token configured, send back symmetric token and
	// register without a lifecycle generation
	case constants.ServerWorkerTokenType:
		w, err := database.FromContext(c).CreateWorker(ctx, input)
		if err != nil {
			util.HandleError(c, createWorkerErrorStatus(err), fmt.Errorf("unable to create worker: %w", err))

			return
		}

		l.WithFields(logrus.Fields{
			"worker":    w.GetHostname(),
			"worker_id": w.GetID(),
		}).Info("worker created")

		if secret, ok := c.Value("secret").(string); ok {
			tkn := new(types.Token)
			tkn.SetToken(secret)

			c.JSON(http.StatusCreated, tkn)

			return
		}

		retErr := fmt.Errorf("symmetric token provided but not configured in server")
		util.HandleError(c, http.StatusBadRequest, retErr)

		return

	// a worker register token establishes exactly one worker generation
	default:
		// the register token must carry the generation it is authorized
		// to establish; tokens issued before generations existed cannot
		// establish a bound generation
		if len(cl.Generation) == 0 {
			retErr := fmt.Errorf("unable to add worker; registration token has no generation")

			util.HandleError(c, http.StatusUnauthorized, retErr)

			return
		}

		// verify the server-side, single-use registration record exists,
		// is still active and unexpired, and belongs to this hostname.
		// this is what prevents replayed or revoked register tokens from
		// establishing generations after a delete/recreate cycle
		registerToken, err := database.FromContext(c).GetWorkerRegisterToken(ctx, cl.Generation)
		if err != nil {
			l.Warnf("rejected registration for %s with unknown generation %s", input.GetHostname(), cl.Generation)

			retErr := fmt.Errorf("unable to add worker; invalid registration token: %w", err)

			util.HandleError(c, http.StatusUnauthorized, retErr)

			return
		}

		if registerToken.GetStatus() != constants.WorkerRegisterTokenStatusActive ||
			!strings.EqualFold(registerToken.GetHostname(), input.GetHostname()) ||
			registerToken.GetExpiresAt() <= time.Now().Unix() {
			l.WithFields(logrus.Fields{
				"status": registerToken.GetStatus(),
			}).Warnf("rejected registration for %s with consumed, expired, or mismatched generation %s", input.GetHostname(), cl.Generation)

			retErr := fmt.Errorf("unable to add worker; registration token is consumed, expired, or not valid for worker %s", input.GetHostname())

			util.HandleError(c, http.StatusUnauthorized, retErr)

			return
		}

		// bind the new worker record to this generation
		input.SetGeneration(cl.Generation)

		tm := c.MustGet("token-manager").(*token.Manager)

		wmto := &token.MintTokenOpts{
			TokenType:     constants.WorkerAuthTokenType,
			TokenDuration: tm.WorkerAuthTokenDuration,
			Hostname:      cl.Subject,
			Generation:    cl.Generation,
		}

		// mint the auth token before the worker record is created. if
		// minting fails, no incomplete generation is left behind holding
		// the hostname, and the worker can simply retry registration
		wt, err := tm.MintToken(wmto)
		if err != nil {
			retErr := fmt.Errorf("unable to generate auth token for worker %s: %w", input.GetHostname(), err)

			util.HandleError(c, http.StatusInternalServerError, retErr)

			return
		}

		// create the worker and consume the register token atomically
		w, err := database.FromContext(c).CreateWorker(ctx, input)
		if err != nil {
			util.HandleError(c, createWorkerErrorStatus(err), fmt.Errorf("unable to create worker: %w", err))

			return
		}

		l.WithFields(logrus.Fields{
			"worker":     w.GetHostname(),
			"worker_id":  w.GetID(),
			"generation": w.GetGeneration(),
		}).Info("worker created")

		tkn := new(types.Token)
		tkn.SetToken(wt)

		c.JSON(http.StatusCreated, tkn)
	}
}

// createWorkerErrorStatus maps worker creation failures to the
// appropriate HTTP status. An invalid or consumed register token and a
// superseded generation are client errors; a hostname already in use
// is a conflict; everything else is a server error.
func createWorkerErrorStatus(err error) int {
	switch {
	case errors.Is(err, dbworker.ErrInvalidWorkerRegisterToken):
		return http.StatusUnauthorized
	case isDuplicateKeyError(err):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// isDuplicateKeyError reports whether the error is a unique-constraint
// violation from either supported database driver.
func isDuplicateKeyError(err error) bool {
	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "duplicate key value violates unique constraint") ||
		strings.Contains(msg, "unique constraint failed")
}
