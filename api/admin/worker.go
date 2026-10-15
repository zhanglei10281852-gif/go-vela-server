// SPDX-License-Identifier: Apache-2.0

package admin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database"
	"github.com/go-vela/server/internal/token"
	"github.com/go-vela/server/util"
)

// swagger:operation POST /api/v1/admin/workers/{worker}/register admin RegisterToken
//
// Get a worker registration token
//
// ---
// produces:
// - application/json
// parameters:
// - in: path
//   name: worker
//   description: Hostname of the worker
//   required: true
//   type: string
// security:
//   - ApiKeyAuth: []
// responses:
//   '200':
//     description: Successfully generated registration token
//     schema:
//       "$ref": "#/definitions/Token"
//   '401':
//     description: Unauthorized
//     schema:
//       "$ref": "#/definitions/Error"
//   '500':
//     description: Unexpected server error
//     schema:
//       "$ref": "#/definitions/Error"

// RegisterToken represents the API handler to
// generate a registration token for onboarding a worker.
func RegisterToken(c *gin.Context) {
	// capture middleware values
	l := c.MustGet("logger").(*logrus.Entry)
	ctx := c.Request.Context()

	// capture middleware values
	host := util.PathParameter(c, "worker")

	// a register token authorizes exactly one registration, which
	// establishes exactly one worker lifecycle generation. the random
	// token identifier is the generation identifier recorded on the
	// worker row when registration succeeds and embedded in every auth
	// token minted for that generation.
	tokenID, err := uuid.NewV7()
	if err != nil {
		retErr := fmt.Errorf("unable to generate worker registration identifier: %w", err)

		util.HandleError(c, http.StatusInternalServerError, retErr)

		return
	}

	tm := c.MustGet("token-manager").(*token.Manager)

	now := time.Now()

	// persist the single-use registration before handing out the token
	registerToken := new(types.WorkerRegisterToken)
	registerToken.SetTokenID(tokenID.String())
	registerToken.SetHostname(host)
	registerToken.SetStatus(constants.WorkerRegisterTokenStatusActive)
	registerToken.SetCreatedAt(now.Unix())
	registerToken.SetExpiresAt(now.Add(tm.WorkerRegisterTokenDuration).Unix())

	_, err = database.FromContext(c).CreateWorkerRegisterToken(ctx, registerToken)
	if err != nil {
		retErr := fmt.Errorf("unable to store worker registration token: %w", err)

		util.HandleError(c, http.StatusInternalServerError, retErr)

		return
	}

	rmto := &token.MintTokenOpts{
		Hostname:      host,
		TokenType:     constants.WorkerRegisterTokenType,
		TokenDuration: tm.WorkerRegisterTokenDuration,
		Generation:    tokenID.String(),
	}

	l.Debug("platform admin: generating worker registration token")

	rt, err := tm.MintToken(rmto)
	if err != nil {
		// do not leave a dangling registration the worker can never
		// complete; the admin can simply request a new token
		if dErr := database.FromContext(c).DeleteWorkerRegisterToken(ctx, tokenID.String()); dErr != nil {
			l.WithError(dErr).Warn("unable to remove worker registration record after mint failure")
		}

		retErr := fmt.Errorf("unable to generate registration token: %w", err)

		util.HandleError(c, http.StatusInternalServerError, retErr)

		return
	}

	l.WithFields(logrus.Fields{
		"worker":     host,
		"generation": tokenID.String(),
	}).Info("platform admin: generated worker registration token")

	c.JSON(http.StatusOK, types.Token{Token: &rt})
}
