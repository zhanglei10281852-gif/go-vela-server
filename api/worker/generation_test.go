// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	apitypes "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database"
	"github.com/go-vela/server/internal/token"
	"github.com/go-vela/server/router/middleware/claims"
	"github.com/go-vela/server/router/middleware/perm"
	usermw "github.com/go-vela/server/router/middleware/user"
	wmw "github.com/go-vela/server/router/middleware/worker"
)

func testTokenManager() *token.Manager {
	return &token.Manager{
		PrivateKeyHMAC:              "worker-generation-test-hmac-secret",
		WorkerAuthTokenDuration:     time.Hour,
		WorkerRegisterTokenDuration: time.Hour,
	}
}

// generationRouter wires the real worker middleware chain against an
// in-memory database and the provided (optional) symmetric secret.
func generationRouter(db database.Interface, tm *token.Manager, secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("logger", logrus.NewEntry(logrus.StandardLogger())) })
	r.Use(func(c *gin.Context) { database.ToContext(c, db) })
	r.Use(func(c *gin.Context) { c.Set("token-manager", tm) })

	if len(secret) > 0 {
		r.Use(func(c *gin.Context) { c.Set("secret", secret) })
	}

	r.Use(claims.Establish())
	r.Use(usermw.Establish())

	r.POST("/workers", perm.MustWorkerRegisterToken(), CreateWorker)

	_worker := r.Group("/workers/:worker")
	_worker.PUT("", perm.MustWorkerAuthToken(), wmw.Establish(), UpdateWorker)
	_worker.POST("/refresh", perm.MustWorkerAuthToken(), wmw.Establish(), Refresh)
	_worker.DELETE("", perm.MustPlatformAdmin(), wmw.Establish(), DeleteWorker)

	return r
}

// issueRegisterToken stores a single-use server-side registration and
// returns its generation and signed register token.
func issueRegisterToken(t *testing.T, db database.Interface, tm *token.Manager, host string) (string, string) {
	t.Helper()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("unable to create register token id: %v", err)
	}

	now := time.Now()

	rt := new(apitypes.WorkerRegisterToken)
	rt.SetTokenID(id.String())
	rt.SetHostname(host)
	rt.SetStatus(constants.WorkerRegisterTokenStatusActive)
	rt.SetCreatedAt(now.Unix())
	rt.SetExpiresAt(now.Add(tm.WorkerRegisterTokenDuration).Unix())

	_, err = db.CreateWorkerRegisterToken(context.Background(), rt)
	if err != nil {
		t.Fatalf("unable to create register token: %v", err)
	}

	jwt, err := tm.MintToken(&token.MintTokenOpts{
		Hostname:      host,
		TokenType:     constants.WorkerRegisterTokenType,
		TokenDuration: tm.WorkerRegisterTokenDuration,
		Generation:    id.String(),
	})
	if err != nil {
		t.Fatalf("unable to mint register token: %v", err)
	}

	return id.String(), jwt
}

func generationRequest(t *testing.T, r *gin.Engine, method, path, bearer string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader

	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("unable to marshal body: %v", err)
		}

		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, path, reader)
	if err != nil {
		t.Fatalf("unable to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if len(bearer) > 0 {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

func authTokenFromResponse(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	tkn := new(apitypes.Token)

	err := json.Unmarshal(w.Body.Bytes(), tkn)
	if err != nil {
		t.Fatalf("unable to decode token response %d %s: %v", w.Code, w.Body.String(), err)
	}

	return tkn.GetToken()
}

// TestWorker_GenerationFlow is an end-to-end check that a registration
// establishes one generation, its register and auth tokens cannot be
// replayed after a delete and recreate, and concurrent stale writes are
// rejected without affecting the new worker.
func TestWorker_GenerationFlow(t *testing.T) {
	// setup database and router
	db, err := database.NewTest()
	if err != nil {
		t.Fatalf("unable to create test database engine: %v", err)
	}
	defer db.Close()

	tm := testTokenManager()
	r := generationRouter(db, tm, "")

	host := "e2e-worker-gen"

	defer func() {
		if w, _ := db.GetWorkerForHostname(context.Background(), host); w != nil {
			_ = db.DeleteWorker(context.Background(), w)
		}
	}()

	// 1. register with a fresh register token
	generationOne, registerOne := issueRegisterToken(t, db, tm, host)

	body := map[string]any{
		"hostname": host,
		"address":  "tcp://10.0.0.1:8080",
		"routes":   []string{"vela"},
		"active":   true,
	}

	w := generationRequest(t, r, http.MethodPost, "/workers", registerOne, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("registration returned %d: %s", w.Code, w.Body.String())
	}

	authOne := authTokenFromResponse(t, w)

	cl, err := tm.ParseToken(authOne)
	if err != nil {
		t.Fatalf("unable to parse returned auth token: %v", err)
	}

	if cl.TokenType != constants.WorkerAuthTokenType ||
		cl.Subject != host ||
		cl.Generation != generationOne {
		t.Fatalf("auth token claims are %+v, want WorkerAuth for %s generation %s", cl, host, generationOne)
	}

	// the worker record is bound to the generation
	stored, err := db.GetWorkerForHostname(context.Background(), host)
	if err != nil {
		t.Fatalf("unable to get stored worker: %v", err)
	}

	if stored.GetGeneration() != generationOne {
		t.Fatalf("stored worker generation is %q, want %q", stored.GetGeneration(), generationOne)
	}

	// 2. the register token cannot establish the generation twice
	w = generationRequest(t, r, http.MethodPost, "/workers", registerOne, body)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("register token replay returned %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// 3. a register token can never update or refresh a worker
	w = generationRequest(t, r, http.MethodPut, "/workers/"+host, registerOne, map[string]any{"address": "tcp://evil"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("register token PUT returned %d, want %d", w.Code, http.StatusUnauthorized)
	}

	w = generationRequest(t, r, http.MethodPost, "/workers/"+host+"/refresh", registerOne, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("register token refresh returned %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// 4. a register token whose generation was never recorded is rejected
	unknownID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("unable to create unknown generation id: %v", err)
	}

	registerUnknown, err := tm.MintToken(&token.MintTokenOpts{
		Hostname:      host,
		TokenType:     constants.WorkerRegisterTokenType,
		TokenDuration: tm.WorkerRegisterTokenDuration,
		Generation:    unknownID.String(),
	})
	if err != nil {
		t.Fatalf("unable to mint unknown register token: %v", err)
	}

	w = generationRequest(t, r, http.MethodPost, "/workers", registerUnknown, body)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unknown register token returned %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// 5. normal update and refresh with the current auth token work
	w = generationRequest(t, r, http.MethodPut, "/workers/"+host, authOne, map[string]any{"address": "tcp://10.0.0.2:8080"})
	if w.Code != http.StatusOK {
		t.Fatalf("auth token PUT returned %d: %s", w.Code, w.Body.String())
	}

	stored, _ = db.GetWorkerForHostname(context.Background(), host)
	if stored.GetAddress() != "tcp://10.0.0.2:8080" {
		t.Errorf("address after PUT is %s", stored.GetAddress())
	}

	w = generationRequest(t, r, http.MethodPost, "/workers/"+host+"/refresh", authOne, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("auth token refresh returned %d: %s", w.Code, w.Body.String())
	}

	refreshed := authTokenFromResponse(t, w)

	refreshedClaims, err := tm.ParseToken(refreshed)
	if err != nil {
		t.Fatalf("unable to parse refreshed auth token: %v", err)
	}

	if refreshedClaims.Generation != generationOne {
		t.Errorf("refreshed token generation is %q, want %q", refreshedClaims.Generation, generationOne)
	}

	// 6. a legacy generation-less auth token updates unbound workers but
	// must be rejected by the generation-bound worker
	legacyHost := "e2e-worker-legacy"

	legacy := new(apitypes.Worker)
	legacy.SetHostname(legacyHost)
	legacy.SetAddress("tcp://10.0.0.3:8080")

	_, err = db.CreateWorker(context.Background(), legacy)
	if err != nil {
		t.Fatalf("unable to create legacy worker: %v", err)
	}

	legacyAuth, err := tm.MintToken(&token.MintTokenOpts{
		Hostname:      legacyHost,
		TokenType:     constants.WorkerAuthTokenType,
		TokenDuration: tm.WorkerAuthTokenDuration,
	})
	if err != nil {
		t.Fatalf("unable to mint legacy auth token: %v", err)
	}

	w = generationRequest(t, r, http.MethodPut, "/workers/"+legacyHost, legacyAuth, map[string]any{"address": "tcp://10.0.0.4:8080"})
	if w.Code != http.StatusOK {
		t.Errorf("legacy auth token PUT returned %d: %s", w.Code, w.Body.String())
	}

	w = generationRequest(t, r, http.MethodPut, "/workers/"+host, legacyAuth, map[string]any{"address": "tcp://evil"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("generation-less token against bound worker returned %d, want %d", w.Code, http.StatusUnauthorized)
	}

	stored, _ = db.GetWorkerForHostname(context.Background(), host)
	if stored.GetAddress() != "tcp://10.0.0.2:8080" {
		t.Errorf("bound worker address changed after rejected legacy update: %s", stored.GetAddress())
	}

	// 7. platform admin deletes the worker
	admin := new(apitypes.User)
	admin.SetName("e2e-generation-admin")
	admin.SetToken("admin-token")
	admin.SetAdmin(true)

	_, err = db.CreateUser(context.Background(), admin)
	if err != nil {
		t.Fatalf("unable to create admin user: %v", err)
	}

	adminToken, err := tm.MintToken(&token.MintTokenOpts{
		User:          admin,
		TokenType:     constants.UserAccessTokenType,
		TokenDuration: time.Hour,
	})
	if err != nil {
		t.Fatalf("unable to mint admin token: %v", err)
	}

	w = generationRequest(t, r, http.MethodDelete, "/workers/"+host, adminToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE worker returned %d: %s", w.Code, w.Body.String())
	}

	// register tokens and worker row are gone
	if _, err := db.GetWorkerRegisterToken(context.Background(), generationOne); err == nil {
		t.Errorf("register token %s should be revoked after delete", generationOne)
	}

	if _, err := db.GetWorkerForHostname(context.Background(), host); err == nil {
		t.Errorf("worker %s should not exist after delete", host)
	}

	// old credentials cannot act once the worker is gone
	w = generationRequest(t, r, http.MethodPut, "/workers/"+host, authOne, map[string]any{"address": "tcp://evil"})
	if w.Code != http.StatusNotFound {
		t.Errorf("old auth PUT after delete returned %d, want %d", w.Code, http.StatusNotFound)
	}

	w = generationRequest(t, r, http.MethodPost, "/workers/"+host+"/refresh", authOne, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("old auth refresh after delete returned %d, want %d", w.Code, http.StatusNotFound)
	}

	// old register token cannot recreate the worker
	w = generationRequest(t, r, http.MethodPost, "/workers", registerOne, body)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("old register token after delete returned %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// 8. a new registration establishes a replacement generation
	generationTwo, registerTwo := issueRegisterToken(t, db, tm, host)

	replacement := map[string]any{
		"hostname": host,
		"address":  "tcp://10.0.1.1:8080",
		"routes":   []string{"vela", "docker"},
		"active":   true,
	}

	w = generationRequest(t, r, http.MethodPost, "/workers", registerTwo, replacement)
	if w.Code != http.StatusCreated {
		t.Fatalf("replacement registration returned %d: %s", w.Code, w.Body.String())
	}

	authTwo := authTokenFromResponse(t, w)

	authTwoClaims, err := tm.ParseToken(authTwo)
	if err != nil {
		t.Fatalf("unable to parse replacement auth token: %v", err)
	}

	if authTwoClaims.Generation != generationTwo {
		t.Errorf("replacement auth generation is %q, want %q", authTwoClaims.Generation, generationTwo)
	}

	if generationTwo == generationOne {
		t.Errorf("replacement generation must differ from the deleted one")
	}

	// 9. every old-generation credential is rejected against the new record
	for _, attempt := range []struct {
		name   string
		method string
		path   string
		token  string
	}{
		{"old auth PUT", http.MethodPut, "/workers/" + host, authOne},
		{"old auth refresh", http.MethodPost, "/workers/" + host + "/refresh", authOne},
		{"old register PUT", http.MethodPut, "/workers/" + host, registerOne},
		{"legacy auth PUT", http.MethodPut, "/workers/" + host, legacyAuth},
	} {
		w = generationRequest(t, r, attempt.method, attempt.path, attempt.token, map[string]any{"address": "tcp://evil"})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s returned %d, want %d", attempt.name, w.Code, http.StatusUnauthorized)
		}
	}

	// the new worker's data must be untouched by all rejected writes
	stored, _ = db.GetWorkerForHostname(context.Background(), host)
	if stored.GetAddress() != "tcp://10.0.1.1:8080" {
		t.Errorf("replacement worker address is %s, want tcp://10.0.1.1:8080", stored.GetAddress())
	}

	if len(stored.GetRoutes()) != 2 || stored.GetRoutes()[0] != "vela" {
		t.Errorf("replacement worker routes are %v, want [vela docker]", stored.GetRoutes())
	}

	if stored.GetGeneration() != generationTwo {
		t.Errorf("replacement worker generation is %q, want %q", stored.GetGeneration(), generationTwo)
	}

	// 10. current-generation update and refresh keep working normally
	w = generationRequest(t, r, http.MethodPut, "/workers/"+host, authTwo, map[string]any{
		"address": "tcp://10.0.1.2:8080",
		"status":  constants.WorkerStatusIdle,
	})
	if w.Code != http.StatusOK {
		t.Errorf("current generation PUT returned %d: %s", w.Code, w.Body.String())
	}

	w = generationRequest(t, r, http.MethodPost, "/workers/"+host+"/refresh", authTwo, nil)
	if w.Code != http.StatusOK {
		t.Errorf("current generation refresh returned %d: %s", w.Code, w.Body.String())
	}
}

// TestWorker_GenerationRecovery ensures a registration whose
// server-side record is missing (for example, one that could not be
// completed) does not hold the hostname, and the worker can retry.
func TestWorker_GenerationRecovery(t *testing.T) {
	// setup database and router
	db, err := database.NewTest()
	if err != nil {
		t.Fatalf("unable to create test database engine: %v", err)
	}
	defer db.Close()

	tm := testTokenManager()
	r := generationRouter(db, tm, "")

	host := "e2e-worker-recover"

	generationOne, registerOne := issueRegisterToken(t, db, tm, host)

	// simulate the registration record being removed before completion
	err = db.DeleteWorkerRegisterToken(context.Background(), generationOne)
	if err != nil {
		t.Fatalf("unable to delete register token: %v", err)
	}

	body := map[string]any{
		"hostname": host,
		"address":  "tcp://10.0.2.1:8080",
		"routes":   []string{"vela"},
		"active":   true,
	}

	// first attempt cannot establish a generation and must not leave a
	// worker behind occupying the hostname
	w := generationRequest(t, r, http.MethodPost, "/workers", registerOne, body)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("registration without a valid record returned %d, want %d", w.Code, http.StatusUnauthorized)
	}

	if _, err := db.GetWorkerForHostname(context.Background(), host); err == nil {
		t.Fatalf("hostname %s should be free after failed registration", host)
	}

	// a fresh registration retries successfully
	_, registerTwo := issueRegisterToken(t, db, tm, host)

	w = generationRequest(t, r, http.MethodPost, "/workers", registerTwo, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("retried registration returned %d: %s", w.Code, w.Body.String())
	}

	stored, err := db.GetWorkerForHostname(context.Background(), host)
	if err != nil {
		t.Fatalf("worker should exist after successful retry: %v", err)
	}

	if len(stored.GetGeneration()) == 0 {
		t.Errorf("retried worker should be bound to a generation")
	}
}

// TestWorker_SymmetricTokenMode verifies the explicitly configured
// shared symmetric server-worker token keeps registering, updating and
// refreshing workers without generation binding.
func TestWorker_SymmetricTokenMode(t *testing.T) {
	// setup database and router
	db, err := database.NewTest()
	if err != nil {
		t.Fatalf("unable to create test database engine: %v", err)
	}
	defer db.Close()

	tm := testTokenManager()
	secret := "super-secret-symmetric-token"
	r := generationRouter(db, tm, secret)

	host := "e2e-worker-symmetric"

	body := map[string]any{
		"hostname": host,
		"address":  "tcp://10.0.3.1:8080",
		"routes":   []string{"vela"},
		"active":   true,
	}

	// registration with the symmetric token returns that same token and
	// does not bind a generation
	w := generationRequest(t, r, http.MethodPost, "/workers", secret, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("symmetric registration returned %d: %s", w.Code, w.Body.String())
	}

	if authTokenFromResponse(t, w) != secret {
		t.Errorf("symmetric registration should return the shared secret")
	}

	stored, err := db.GetWorkerForHostname(context.Background(), host)
	if err != nil {
		t.Fatalf("unable to get symmetric worker: %v", err)
	}

	if stored.GetGeneration() != "" {
		t.Errorf("symmetric worker generation is %q, want empty", stored.GetGeneration())
	}

	// updates and refresh with the shared token continue to work
	w = generationRequest(t, r, http.MethodPut, "/workers/"+host, secret, map[string]any{"address": "tcp://10.0.3.2:8080"})
	if w.Code != http.StatusOK {
		t.Errorf("symmetric PUT returned %d: %s", w.Code, w.Body.String())
	}

	w = generationRequest(t, r, http.MethodPost, "/workers/"+host+"/refresh", secret, nil)
	if w.Code != http.StatusOK {
		t.Errorf("symmetric refresh returned %d: %s", w.Code, w.Body.String())
	}

	if authTokenFromResponse(t, w) != secret {
		t.Errorf("symmetric refresh should return the shared secret")
	}
}
