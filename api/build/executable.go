// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/go-vela/server/database"
	"github.com/go-vela/server/router/middleware/build"
	"github.com/go-vela/server/router/middleware/repo"
	"github.com/go-vela/server/util"
)

// swagger:operation GET /api/v1/repos/{org}/{repo}/builds/{build}/executable builds GetBuildExecutable
//
// Get a build executable
//
// ---
// produces:
// - application/json
// parameters:
// - in: path
//   name: org
//   description: Name of the organization
//   required: true
//   type: string
// - in: path
//   name: repo
//   description: Name of the repository
//   required: true
//   type: string
// - in: path
//   name: build
//   description: Build number
//   required: true
//   type: integer
// security:
//   - ApiKeyAuth: []
// responses:
//   '200':
//     description: Successfully retrieved the build executable
//     type: json
//     schema:
//       "$ref": "#/definitions/BuildExecutable"
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

// GetBuildExecutable represents the API handler to get
// a build executable for a repository.
func GetBuildExecutable(c *gin.Context) {
	// capture middleware values
	l := c.MustGet("logger").(*logrus.Entry)
	b := build.Retrieve(c)
	r := repo.Retrieve(c)
	ctx := c.Request.Context()

	l.Debugf("reading build executable %s/%d", r.GetFullName(), b.GetNumber())

	// send database call to pop the requested build executable from the table
	bExecutable, err := database.FromContext(c).PopBuildExecutable(ctx, b.GetID())
	if err != nil {
		retErr := fmt.Errorf("unable to pop build executable: %w", err)
		util.HandleError(c, http.StatusInternalServerError, retErr)

		return
	}

	c.JSON(http.StatusOK, bExecutable)
}
