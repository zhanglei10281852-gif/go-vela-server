// SPDX-License-Identifier: Apache-2.0

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	apitypes "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database"
	"github.com/go-vela/server/internal/token"
)

// TestAdmin_RegisterToken verifies the admin handler records a
// single-use registration scoped to a generation and embeds that
// generation in the signed register token.
func TestAdmin_RegisterToken(t *testing.T) {
	// setup database
	db, err := database.NewTest()
	if err != nil {
		t.Fatalf("unable to create test database engine: %v", err)
	}
	defer db.Close()

	tm := &token.Manager{
		PrivateKeyHMAC:              "admin-register-token-hmac",
		WorkerRegisterTokenDuration: 30 * time.Minute,
	}

	gin.SetMode(gin.TestMode)

	resp := httptest.NewRecorder()
	c, engine := gin.CreateTestContext(resp)
	c.Request, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/workers/my-worker/register", nil)

	engine.Use(func(gc *gin.Context) { gc.Set("logger", logrus.NewEntry(logrus.StandardLogger())) })
	engine.Use(func(gc *gin.Context) { database.ToContext(gc, db) })
	engine.Use(func(gc *gin.Context) { gc.Set("token-manager", tm) })
	engine.POST("/admin/workers/:worker/register", RegisterToken)

	engine.ServeHTTP(resp, c.Request)

	if resp.Code != http.StatusOK {
		t.Fatalf("RegisterToken returned %d: %s", resp.Code, resp.Body.String())
	}

	tkn := new(apitypes.Token)

	err = json.Unmarshal(resp.Body.Bytes(), tkn)
	if err != nil {
		t.Fatalf("unable to decode token response: %v", err)
	}

	claims, err := tm.ParseToken(tkn.GetToken())
	if err != nil {
		t.Fatalf("unable to parse register token: %v", err)
	}

	if claims.TokenType != constants.WorkerRegisterTokenType {
		t.Errorf("token type is %s, want %s", claims.TokenType, constants.WorkerRegisterTokenType)
	}

	if claims.Subject != "my-worker" {
		t.Errorf("token subject is %s, want my-worker", claims.Subject)
	}

	if len(claims.Generation) == 0 {
		t.Fatal("register token should carry a generation")
	}

	// the single-use registration must be recorded and active
	record, err := db.GetWorkerRegisterToken(context.Background(), claims.Generation)
	if err != nil {
		t.Fatalf("register token record not found: %v", err)
	}

	if record.GetHostname() != "my-worker" {
		t.Errorf("record hostname is %s, want my-worker", record.GetHostname())
	}

	if record.GetStatus() != constants.WorkerRegisterTokenStatusActive {
		t.Errorf("record status is %s, want %s", record.GetStatus(), constants.WorkerRegisterTokenStatusActive)
	}

	if record.GetExpiresAt() <= time.Now().Unix() {
		t.Errorf("record expiration %d should be in the future", record.GetExpiresAt())
	}
}
