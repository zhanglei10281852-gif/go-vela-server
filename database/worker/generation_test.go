// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	api "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
)

// newGenerationWorker builds an unpersisted generation-bound worker.
func newGenerationWorker(host, address, generation string) *api.Worker {
	w := new(api.Worker)
	w.SetHostname(host)
	w.SetAddress(address)
	w.SetRoutes([]string{"vela"})
	w.SetActive(true)
	w.SetStatus(constants.WorkerStatusAvailable)
	w.SetGeneration(generation)

	return w
}

// TestWorker_GenerationLifecycle exercises the security properties of
// worker lifecycle generations at the database layer using Sqlite.
func TestWorker_GenerationLifecycle(t *testing.T) {
	// setup types
	_engine := testSqlite(t)

	defer func() { _sql, _ := _engine.client.DB(); _sql.Close() }()

	ctx := context.TODO()
	now := time.Now().Unix()

	// 1. an active register token establishes exactly one generation
	registerToken := new(api.WorkerRegisterToken)
	registerToken.SetTokenID("generation-one")
	registerToken.SetHostname("gen-worker")
	registerToken.SetStatus(constants.WorkerRegisterTokenStatusActive)
	registerToken.SetCreatedAt(now)
	registerToken.SetExpiresAt(now + 3600)

	_, err := _engine.CreateWorkerRegisterToken(ctx, registerToken)
	if err != nil {
		t.Fatalf("unable to create register token: %v", err)
	}

	workerOne := newGenerationWorker("gen-worker", "https://10.0.0.1:8080", "generation-one")

	workerOne, err = _engine.CreateWorker(ctx, workerOne)
	if err != nil {
		t.Fatalf("CreateWorker with a valid register token returned err: %v", err)
	}

	if workerOne.GetGeneration() != "generation-one" {
		t.Errorf("worker generation is %s, want generation-one", workerOne.GetGeneration())
	}

	// the register token must now be marked used and bound to the worker
	consumed, err := _engine.GetWorkerRegisterToken(ctx, "generation-one")
	if err != nil {
		t.Fatalf("unable to get consumed register token: %v", err)
	}

	if consumed.GetStatus() != constants.WorkerRegisterTokenStatusUsed {
		t.Errorf("register token status is %s, want %s", consumed.GetStatus(), constants.WorkerRegisterTokenStatusUsed)
	}

	if consumed.GetWorkerID() != workerOne.GetID() {
		t.Errorf("register token worker id is %d, want %d", consumed.GetWorkerID(), workerOne.GetID())
	}

	// 2. a token scoped to a different hostname cannot establish a generation
	wrongHost := newGenerationWorker("gen-worker-other", "https://10.0.0.2:8080", "generation-one")

	_, err = _engine.CreateWorker(ctx, wrongHost)
	if !errors.Is(err, ErrInvalidWorkerRegisterToken) {
		t.Errorf("CreateWorker with mismatched hostname returned %v, want ErrInvalidWorkerRegisterToken", err)
	}

	// 3. an expired register token is rejected and rolls the worker
	// insert back, leaving the hostname unoccupied
	expiredToken := new(api.WorkerRegisterToken)
	expiredToken.SetTokenID("generation-expired")
	expiredToken.SetHostname("gen-worker-expired")
	expiredToken.SetStatus(constants.WorkerRegisterTokenStatusActive)
	expiredToken.SetCreatedAt(now - 7200)
	expiredToken.SetExpiresAt(now - 1)

	_, err = _engine.CreateWorkerRegisterToken(ctx, expiredToken)
	if err != nil {
		t.Fatalf("unable to create expired register token: %v", err)
	}

	expiredWorker := newGenerationWorker("gen-worker-expired", "https://10.0.0.3:8080", "generation-expired")

	_, err = _engine.CreateWorker(ctx, expiredWorker)
	if !errors.Is(err, ErrInvalidWorkerRegisterToken) {
		t.Errorf("CreateWorker with expired token returned %v, want ErrInvalidWorkerRegisterToken", err)
	}

	_, err = _engine.GetWorkerForHostname(ctx, "gen-worker-expired")
	if err == nil {
		t.Errorf("worker gen-worker-expired should not exist after rolled back registration")
	}

	// 4. an unknown register token is rejected and leaves no worker behind
	unknownWorker := newGenerationWorker("gen-worker-unknown", "https://10.0.0.4:8080", "generation-unknown")

	_, err = _engine.CreateWorker(ctx, unknownWorker)
	if !errors.Is(err, ErrInvalidWorkerRegisterToken) {
		t.Errorf("CreateWorker with unknown token returned %v, want ErrInvalidWorkerRegisterToken", err)
	}

	_, err = _engine.GetWorkerForHostname(ctx, "gen-worker-unknown")
	if err == nil {
		t.Errorf("worker gen-worker-unknown should not exist after rejected registration")
	}

	// 5. deleting the worker revokes every register token for the hostname;
	// the old generation can no longer establish or act on the worker
	err = _engine.DeleteWorker(ctx, workerOne)
	if err != nil {
		t.Fatalf("DeleteWorker returned err: %v", err)
	}

	_, err = _engine.GetWorkerRegisterToken(ctx, "generation-one")
	if err == nil {
		t.Errorf("register token generation-one should be revoked after worker delete")
	}

	_, err = _engine.GetWorkerForHostname(ctx, "gen-worker")
	if err == nil {
		t.Errorf("worker gen-worker should be deleted")
	}

	// recreating with the revoked register token must fail atomically
	replayWorker := newGenerationWorker("gen-worker", "https://10.0.0.1:8080", "generation-one")

	_, err = _engine.CreateWorker(ctx, replayWorker)
	if !errors.Is(err, ErrInvalidWorkerRegisterToken) {
		t.Errorf("CreateWorker with revoked token returned %v, want ErrInvalidWorkerRegisterToken", err)
	}

	// the failed replay must not occupy the hostname
	_, err = _engine.GetWorkerForHostname(ctx, "gen-worker")
	if err == nil {
		t.Errorf("worker gen-worker should not exist after rejected replay")
	}

	// 6. a fresh register token establishes the replacement generation
	newToken := new(api.WorkerRegisterToken)
	newToken.SetTokenID("generation-two")
	newToken.SetHostname("gen-worker")
	newToken.SetStatus(constants.WorkerRegisterTokenStatusActive)
	newToken.SetCreatedAt(now)
	newToken.SetExpiresAt(now + 3600)

	_, err = _engine.CreateWorkerRegisterToken(ctx, newToken)
	if err != nil {
		t.Fatalf("unable to create replacement register token: %v", err)
	}

	workerTwo := newGenerationWorker("gen-worker", "https://10.0.0.9:8080", "generation-two")

	workerTwo, err = _engine.CreateWorker(ctx, workerTwo)
	if err != nil {
		t.Fatalf("CreateWorker replacement generation returned err: %v", err)
	}

	if workerTwo.GetID() == workerOne.GetID() {
		t.Errorf("replacement worker id %d must differ from deleted id %d", workerTwo.GetID(), workerOne.GetID())
	}

	// 7. a late update carrying the old generation is conditionally
	// rejected; it must never overwrite the new worker's data
	stale := newGenerationWorker("gen-worker", "https://evil.override:8080", "generation-one")
	stale.SetID(workerOne.GetID())

	_, err = _engine.UpdateWorker(ctx, stale)
	if !errors.Is(err, ErrStaleWorker) {
		t.Errorf("UpdateWorker from an old generation returned %v, want ErrStaleWorker", err)
	}

	current, err := _engine.GetWorkerForHostname(ctx, "gen-worker")
	if err != nil {
		t.Fatalf("unable to get current worker: %v", err)
	}

	if current.GetAddress() != "https://10.0.0.9:8080" {
		t.Errorf("new worker address was overwritten by a stale update: %s", current.GetAddress())
	}

	if current.GetGeneration() != "generation-two" {
		t.Errorf("new worker generation is %s, want generation-two", current.GetGeneration())
	}

	// 8. the current generation updates normally
	current.SetAddress("https://10.0.0.10:8080")

	current, err = _engine.UpdateWorker(ctx, current)
	if err != nil {
		t.Errorf("UpdateWorker for current generation returned err: %v", err)
	}

	if current.GetAddress() != "https://10.0.0.10:8080" {
		t.Errorf("updated address is %s, want https://10.0.0.10:8080", current.GetAddress())
	}

	// 9. a double delete is reported as stale
	err = _engine.DeleteWorker(ctx, workerOne)
	if !errors.Is(err, ErrStaleWorker) {
		t.Errorf("second DeleteWorker returned %v, want ErrStaleWorker", err)
	}

	// 10. legacy workers (no generation) keep updating on ID only
	legacy := new(api.Worker)
	legacy.SetHostname("gen-worker-legacy")
	legacy.SetAddress("https://10.0.0.5:8080")

	legacy, err = _engine.CreateWorker(ctx, legacy)
	if err != nil {
		t.Fatalf("CreateWorker legacy returned err: %v", err)
	}

	legacy.SetStatus(constants.WorkerStatusIdle)

	_, err = _engine.UpdateWorker(ctx, legacy)
	if err != nil {
		t.Errorf("UpdateWorker legacy returned err: %v", err)
	}
}
