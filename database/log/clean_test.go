// SPDX-License-Identifier: Apache-2.0

package log

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database/testutils"
)

// pgify mirrors the gorm postgres dialector rewrite of "?" placeholders into
// "$1, $2, ..." so expectations match the exact text sent to the driver.
func pgify(query string) string {
	var b strings.Builder

	n := 0

	for _, r := range query {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
		} else {
			b.WriteRune(r)
		}
	}

	return b.String()
}

func cleanupLeaseColumns() []string {
	return []string{
		"id", "generation", "epoch", "owner_id", "leased_until", "status", "before",
		"batch_size", "with_vacuum", "mode", "cursor_id", "partitions", "partition_index",
		"partition_cursor_id", "affected_partitions", "deleted_count", "vacuum_done",
		"last_error", "created_at", "updated_at",
	}
}

func newTestLease(before int64, batchSize int64, withVacuum bool) *cleanupLease {
	now := time.Now().Unix()

	return &cleanupLease{
		ID:                 1,
		Generation:         1,
		Epoch:              1,
		OwnerID:            "owner-1",
		LeasedUntil:        now + 600,
		Status:             cleanupStatusRunning,
		Before:             before,
		BatchSize:          batchSize,
		WithVacuum:         withVacuum,
		Mode:               "",
		CursorID:           0,
		Partitions:         "[]",
		PartitionIndex:     0,
		PartitionCursorID:  0,
		AffectedPartitions: "[]",
		DeletedCount:       0,
		VacuumDone:         false,
		LastError:          "",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func leaseRow(lease *cleanupLease) *sqlmock.Rows {
	return sqlmock.NewRows(cleanupLeaseColumns()).AddRow(
		lease.ID, lease.Generation, lease.Epoch, lease.OwnerID, lease.LeasedUntil,
		lease.Status, lease.Before, lease.BatchSize, lease.WithVacuum, lease.Mode,
		lease.CursorID, lease.Partitions, lease.PartitionIndex, lease.PartitionCursorID,
		lease.AffectedPartitions, lease.DeletedCount, lease.VacuumDone, lease.LastError,
		lease.CreatedAt, lease.UpdatedAt,
	)
}

func idRows(ids ...int64) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}

	return rows
}

// expectLeaseTableCreate expects the coordination table DDL.
func expectLeaseTableCreate(mock sqlmock.Sqlmock) {
	mock.ExpectExec(createCleanupLeaseTable).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// expectLeaseInsert expects the empty read and the first-generation
// insert/returning acquisition.
func expectLeaseInsert(mock sqlmock.Sqlmock, lease *cleanupLease) {
	mock.ExpectQuery(pgify(selectCleanupLeaseSQL)).
		WithArgs(testutils.AnyArgument{}).
		WillReturnRows(sqlmock.NewRows(cleanupLeaseColumns()))

	mock.ExpectQuery(pgify(insertCleanupLeaseSQL)).
		WithArgs(
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{},
		).
		WillReturnRows(leaseRow(lease))
}

// expectTraditionalMode expects the owner-gated mode selection.
func expectTraditionalMode(mock sqlmock.Sqlmock) {
	mock.ExpectExec(pgify(setCleanupModeSQL)).
		WithArgs(
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectProgressExec expects a committed batch progress / checkpoint update.
func expectProgressExec(mock sqlmock.Sqlmock) {
	mock.ExpectExec(pgify(updateCleanupProgressSQL)).
		WithArgs(
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectBatch expects a transaction that deletes one keyset batch and commits
// its progress.
func expectBatch(mock sqlmock.Sqlmock, table string, ids []int64, queryErr error) {
	query := pgify(fmt.Sprintf(deleteCleanupBatchSQLTemplate, table, table))

	mock.ExpectBegin()

	if queryErr != nil {
		mock.ExpectQuery(query).
			WithArgs(testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{}).
			WillReturnError(queryErr)
		mock.ExpectRollback()

		return
	}

	mock.ExpectQuery(query).
		WithArgs(testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{}).
		WillReturnRows(idRows(ids...))
	expectProgressExec(mock)
	mock.ExpectCommit()
}

func expectFinalize(mock sqlmock.Sqlmock) {
	mock.ExpectExec(pgify(finalizeCleanupSQL)).
		WithArgs(
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// testSqliteFile creates an isolated, file backed SQLite engine for
// deterministic integration behavior within this package.
func testSqliteFile(t *testing.T) *Engine {
	t.Helper()

	client, err := gorm.Open(
		sqlite.Open(filepath.Join(t.TempDir(), "logs.db")),
		&gorm.Config{SkipDefaultTransaction: true},
	)
	if err != nil {
		t.Errorf("unable to create new sqlite database: %v", err)
	}

	engine, err := New(
		WithClient(client),
		WithLogger(logrus.NewEntry(logrus.StandardLogger())),
		WithSkipCreation(false),
	)
	if err != nil {
		t.Errorf("unable to create new sqlite log engine: %v", err)
	}

	t.Cleanup(func() {
		if sqlDB, err := client.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return engine
}

func rangeIDs(start, end int64) []int64 {
	ids := make([]int64, 0, end-start+1)

	for id := start; id <= end; id++ {
		ids = append(ids, id)
	}

	return ids
}

func TestLog_Engine_CleanLogs(t *testing.T) {
	// setup types
	cutoffTime := time.Now().Add(-48 * time.Hour).Unix()

	// setup the test database client
	_postgres, _mock := testPostgres(t)

	defer func() { _sql, _ := _postgres.client.DB(); _sql.Close() }()

	lease := newTestLease(cutoffTime, 1000, true)

	expectLeaseTableCreate(_mock)
	expectLeaseInsert(_mock, lease)
	expectTraditionalMode(_mock)

	// first batch: delete 1000 records
	expectBatch(_mock, constants.TableLog, rangeIDs(1, 1000), nil)
	// second batch: delete 500 records (less than batch size, indicating we're done)
	expectBatch(_mock, constants.TableLog, rangeIDs(1001, 1500), nil)

	// vacuum gate checkpoint
	expectProgressExec(_mock)
	// VACUUM operation
	_mock.ExpectExec(`VACUUM ANALYZE logs`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// vacuum persisted
	_mock.ExpectExec(pgify(setVacuumDoneSQL)).
		WithArgs(
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// generation completed
	expectFinalize(_mock)

	_sqlite := testSqliteFile(t)

	// create test logs in sqlite for integration testing
	for i := int64(1); i <= 10; i++ {
		err := _sqlite.client.Exec(
			"INSERT INTO logs (id, build_id, repo_id, service_id, step_id, data, created_at) VALUES (?, 1, 1, ?, ?, ?, ?)",
			i, i+100, i, []byte("test log data"), cutoffTime-int64(i*3600),
		).Error
		if err != nil {
			t.Errorf("unable to create test log for sqlite: %v", err)
		}
	}

	// setup tests
	tests := []struct {
		failure     bool
		name        string
		database    *Engine
		before      int64
		batchSize   int
		withVacuum  bool
		driver      string
		wantDeleted int64
	}{
		{
			failure:     false,
			name:        "postgres with vacuum",
			database:    _postgres,
			before:      cutoffTime,
			batchSize:   1000,
			withVacuum:  true,
			driver:      constants.DriverPostgres,
			wantDeleted: 1500,
		},
		{
			failure:     false,
			name:        "sqlite without vacuum",
			database:    _sqlite,
			before:      cutoffTime,
			batchSize:   5,
			withVacuum:  false,
			driver:      constants.DriverSqlite,
			wantDeleted: 10,
		},
	}

	// run tests
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.database.CleanLogs(context.TODO(), test.before, test.batchSize, test.withVacuum, test.driver)

			if test.failure {
				if err == nil {
					t.Errorf("CleanLogs for %s should have returned err", test.name)
				}

				return
			}

			if err != nil {
				t.Errorf("CleanLogs for %s returned err: %v", test.name, err)
			}

			if result == nil {
				t.Errorf("CleanLogs for %s returned nil result", test.name)
				return
			}

			if !reflect.DeepEqual(result.DeletedCount, test.wantDeleted) {
				t.Errorf("CleanLogs for %s returned %d deleted, want %d", test.name, result.DeletedCount, test.wantDeleted)
			}

			// For traditional mode (non-partitioned), affected partitions should be empty
			if len(result.AffectedPartitions) != 0 {
				t.Errorf("CleanLogs for %s returned %d affected partitions, want 0 for traditional mode", test.name, len(result.AffectedPartitions))
			}
		})
	}

	if err := _mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sql expectations: %v", err)
	}
}

func TestLog_Engine_CleanLogs_EmptyDatabase(t *testing.T) {
	// setup the test database client
	_postgres, _mock := testPostgres(t)

	defer func() { _sql, _ := _postgres.client.DB(); _sql.Close() }()

	cutoffTime := time.Now().Add(-48 * time.Hour).Unix()

	lease := newTestLease(cutoffTime, 1000, false)

	expectLeaseTableCreate(_mock)
	expectLeaseInsert(_mock, lease)
	expectTraditionalMode(_mock)

	// single empty batch: no records deleted, progress check still fences
	expectBatch(_mock, constants.TableLog, []int64{}, nil)

	// generation completed, no vacuum
	expectFinalize(_mock)

	deleted, err := _postgres.CleanLogs(context.TODO(), cutoffTime, 1000, false, constants.DriverPostgres)
	if err != nil {
		t.Errorf("CleanLogs should not have returned err: %v", err)
	}

	if deleted == nil {
		t.Errorf("CleanLogs returned nil result")
		return
	}

	if deleted.DeletedCount != 0 {
		t.Errorf("CleanLogs returned %d deleted, want 0", deleted.DeletedCount)
	}

	if len(deleted.AffectedPartitions) != 0 {
		t.Errorf("CleanLogs returned %d affected partitions, want 0", len(deleted.AffectedPartitions))
	}

	if err := _mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sql expectations: %v", err)
	}
}

func TestLog_Engine_CleanLogs_ContextCancellation(t *testing.T) {
	// setup the test database client
	_postgres, _ := testPostgres(t)

	defer func() { _sql, _ := _postgres.client.DB(); _sql.Close() }()

	cutoffTime := time.Now().Add(-48 * time.Hour).Unix()

	// create a canceled context
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // immediately cancel

	// the function should return early due to context cancellation, without
	// touching the lease table
	result, err := _postgres.CleanLogs(ctx, cutoffTime, 1000, false, constants.DriverPostgres)
	if err == nil {
		t.Errorf("CleanLogs should have returned context cancellation error")
	}

	if result != nil && result.DeletedCount != 0 {
		t.Errorf("CleanLogs returned %d deleted, want 0", result.DeletedCount)
	}
}

func TestLog_Engine_CleanLogs_ActiveLeaseRejected(t *testing.T) {
	_postgres, _mock := testPostgres(t)

	defer func() { _sql, _ := _postgres.client.DB(); _sql.Close() }()

	cutoffTime := time.Now().Add(-48 * time.Hour).Unix()

	lease := newTestLease(cutoffTime, 1000, false)
	lease.OwnerID = "other-live-instance"
	lease.LeasedUntil = time.Now().Add(10 * time.Minute).Unix()
	lease.Mode = cleanupModeTraditional

	expectLeaseTableCreate(_mock)

	// one read returns a live lease; the request must be rejected
	_mock.ExpectQuery(pgify(selectCleanupLeaseSQL)).
		WithArgs(testutils.AnyArgument{}).
		WillReturnRows(leaseRow(lease))

	_, err := _postgres.CleanLogs(context.TODO(), cutoffTime, 1000, false, constants.DriverPostgres)
	if err == nil {
		t.Fatal("CleanLogs should have rejected a live lease")
	}

	if !strings.Contains(err.Error(), "another cleanup operation is already in progress") {
		t.Errorf("CleanLogs returned %v, want in progress error", err)
	}

	if err := _mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sql expectations: %v", err)
	}
}

func TestLog_Engine_CleanLogs_TakeoverResumesFromCursor(t *testing.T) {
	cutoffTime := time.Now().Add(-48 * time.Hour).Unix()

	engine := testSqliteFile(t)

	// create the lease table and seed an expired, interrupted generation that
	// already deleted 3 logs up to cursor 3
	if err := engine.client.Exec(createCleanupLeaseTable).Error; err != nil {
		t.Fatalf("unable to create lease table: %v", err)
	}

	err := engine.client.Exec(
		`INSERT INTO log_cleanup_leases (id, generation, epoch, owner_id, leased_until, status, "before",
			batch_size, with_vacuum, mode, cursor_id, partitions, partition_index, partition_cursor_id,
			affected_partitions, deleted_count, vacuum_done, last_error, created_at, updated_at)
		 VALUES (1, 1, 1, 'dead-instance', 1, 'running', ?, 5, FALSE, 'traditional', 3, '[]', 0, 0, '[]', 3, FALSE, 'interrupted', ?, ?)`,
		cutoffTime, cutoffTime, cutoffTime,
	).Error
	if err != nil {
		t.Fatalf("unable to seed stale lease: %v", err)
	}

	// remaining old logs 4..10 and one recent log 11 that must survive
	for i := int64(4); i <= 10; i++ {
		err = engine.client.Exec(
			"INSERT INTO logs (id, build_id, repo_id, service_id, step_id, data, created_at) VALUES (?, 1, 1, ?, ?, ?, ?)",
			i, i+100, i, []byte("old"), cutoffTime-int64(i),
		).Error
		if err != nil {
			t.Fatalf("unable to seed old log %d: %v", i, err)
		}
	}

	err = engine.client.Exec(
		"INSERT INTO logs (id, build_id, repo_id, service_id, step_id, data, created_at) VALUES (11, 1, 1, 200, 200, ?, ?)",
		[]byte("recent"), time.Now().Unix(),
	).Error
	if err != nil {
		t.Fatalf("unable to seed recent log: %v", err)
	}

	result, err := engine.CleanLogs(context.TODO(), cutoffTime, 5, false, constants.DriverSqlite)
	if err != nil {
		t.Fatalf("CleanLogs takeover returned err: %v", err)
	}

	// the recovered 3 plus the 7 deleted by the new owner, counted exactly once
	if result.DeletedCount != 10 {
		t.Errorf("CleanLogs returned %d deleted, want 10", result.DeletedCount)
	}

	if len(result.AffectedPartitions) != 0 {
		t.Errorf("CleanLogs returned %d affected partitions, want 0", len(result.AffectedPartitions))
	}

	// only the recent log remains
	var remaining int64

	err = engine.client.Raw("SELECT COUNT(*) FROM logs").Scan(&remaining).Error
	if err != nil {
		t.Fatalf("unable to count remaining logs: %v", err)
	}

	if remaining != 1 {
		t.Errorf("found %d remaining logs, want 1 (recent log must survive)", remaining)
	}

	// the same generation was taken over, its progress and completion are durable
	var (
		status       string
		generation   int64
		epoch        int64
		deletedCount int64
		cursor       int64
	)

	err = engine.client.Raw(
		`SELECT status, generation, epoch, deleted_count, cursor_id FROM log_cleanup_leases WHERE id = 1`,
	).Row().Scan(&status, &generation, &epoch, &deletedCount, &cursor)
	if err != nil {
		t.Fatalf("unable to read lease after takeover: %v", err)
	}

	if status != cleanupStatusCompleted {
		t.Errorf("lease status = %s, want %s", status, cleanupStatusCompleted)
	}

	if generation != 1 {
		t.Errorf("generation = %d, want continued generation 1", generation)
	}

	if epoch != 2 {
		t.Errorf("epoch = %d, want bumped epoch 2", epoch)
	}

	if deletedCount != 10 {
		t.Errorf("persisted deleted_count = %d, want 10", deletedCount)
	}

	if cursor != 10 {
		t.Errorf("persisted cursor_id = %d, want 10", cursor)
	}
}

func TestLog_Engine_CleanLogs_FencedOwnerCannotMutateOrRelease(t *testing.T) {
	cutoffTime := time.Now().Add(-48 * time.Hour).Unix()

	engine := testSqliteFile(t)

	if err := engine.client.Exec(createCleanupLeaseTable).Error; err != nil {
		t.Fatalf("unable to create lease table: %v", err)
	}

	// current owner is B at epoch 2; A owned epoch 1 and is late
	err := engine.client.Exec(
		`INSERT INTO log_cleanup_leases (id, generation, epoch, owner_id, leased_until, status, "before",
			batch_size, with_vacuum, mode, cursor_id, partitions, partition_index, partition_cursor_id,
			affected_partitions, deleted_count, vacuum_done, last_error, created_at, updated_at)
		 VALUES (1, 1, 2, 'owner-B', ?, 'running', ?, 5, FALSE, 'traditional', 0, '[]', 0, 0, '[]', 7, FALSE, '', ?, ?)`,
		time.Now().Add(10*time.Minute).Unix(), cutoffTime, cutoffTime, cutoffTime,
	).Error
	if err != nil {
		t.Fatalf("unable to seed lease: %v", err)
	}

	for i := int64(1); i <= 3; i++ {
		err = engine.client.Exec(
			"INSERT INTO logs (id, build_id, repo_id, service_id, step_id, data, created_at) VALUES (?, 1, 1, ?, ?, ?, ?)",
			i, i+100, i, []byte("old"), cutoffTime-i,
		).Error
		if err != nil {
			t.Fatalf("unable to seed log %d: %v", i, err)
		}
	}

	newRun := func(owner string, epoch int64) *cleanupRun {
		run := &cleanupRun{
			ownerID:    owner,
			generation: 1,
			epoch:      epoch,
		}
		run.runCtx, run.cancelRun = context.WithCancel(context.Background())

		return run
	}

	stale := newRun("owner-A", 1)
	lease := newTestLease(cutoffTime, 5, false)

	// a stale owner's batch must be fenced and rolled back: no rows deleted
	n, err := engine.deleteCleanupBatch(stale, lease, constants.TableLog, 0, nil)
	if !errors.Is(err, errCleanupFenced) {
		t.Errorf("stale owner delete returned err %v, want errCleanupFenced", err)
	}

	if n != 0 {
		t.Errorf("stale owner deleted %d rows, want 0 (rollback)", n)
	}

	var remaining int64

	err = engine.client.Raw("SELECT COUNT(*) FROM logs").Scan(&remaining).Error
	if err != nil {
		t.Fatalf("unable to count logs: %v", err)
	}

	if remaining != 3 {
		t.Errorf("found %d logs after fenced delete, want all 3 retained", remaining)
	}

	// a stale owner can not complete the generation
	finalized, err := engine.finalizeCleanupLease(stale)
	if err != nil {
		t.Errorf("stale finalize returned err: %v", err)
	}

	if finalized {
		t.Errorf("stale owner finalized the generation owned by B")
	}

	// a stale owner can not release the new owner's lease
	engine.relinquishCleanupLease(stale, fmt.Errorf("late cancellation"))

	var (
		owner       string
		epoch       int64
		status      string
		leasedUntil int64
	)

	wantUntil := time.Now().Add(10 * time.Minute).Unix()

	err = engine.client.Raw(
		"SELECT owner_id, epoch, status, leased_until FROM log_cleanup_leases WHERE id = 1",
	).Row().Scan(&owner, &epoch, &status, &leasedUntil)
	if err != nil {
		t.Fatalf("unable to read lease: %v", err)
	}

	if owner != "owner-B" || epoch != 2 || status != cleanupStatusRunning {
		t.Errorf("lease mutated by stale owner: owner=%s epoch=%d status=%s", owner, epoch, status)
	}

	if leasedUntil < wantUntil-5 {
		t.Errorf("stale owner expired the live lease: leased_until=%d want~%d", leasedUntil, wantUntil)
	}

	// the current owner deletes successfully
	current := newRun("owner-B", 2)
	lease = newTestLease(cutoffTime, 5, false)

	n, err = engine.deleteCleanupBatch(current, lease, constants.TableLog, 0, nil)
	if err != nil {
		t.Errorf("current owner delete returned err: %v", err)
	}

	if n != 3 {
		t.Errorf("current owner deleted %d rows, want 3", n)
	}

	err = engine.client.Raw("SELECT COUNT(*) FROM logs").Scan(&remaining).Error
	if err != nil {
		t.Fatalf("unable to count logs: %v", err)
	}

	if remaining != 0 {
		t.Errorf("found %d logs after current owner delete, want 0", remaining)
	}
}

func TestLog_Engine_CleanLogs_CanceledRunIsImmediatelyTakeover(t *testing.T) {
	cutoffTime := time.Now().Add(-48 * time.Hour).Unix()

	engine := testSqliteFile(t)

	// seed enough logs that a run with tiny batches is still in flight when the
	// client disconnects
	const totalLogs = 200

	for i := int64(1); i <= totalLogs; i++ {
		err := engine.client.Exec(
			"INSERT INTO logs (id, build_id, repo_id, service_id, step_id, data, created_at) VALUES (?, 1, 1, ?, ?, ?, ?)",
			i, i+1000, i, []byte("old"), cutoffTime-i,
		).Error
		if err != nil {
			t.Fatalf("unable to seed log %d: %v", i, err)
		}
	}

	// first run: cancel the request while batches are still running
	firstCtx, cancelFirst := context.WithCancel(context.Background())

	firstDone := make(chan error, 1)

	go func() {
		_, err := engine.CleanLogs(firstCtx, cutoffTime, 10, false, constants.DriverSqlite)
		firstDone <- err
	}()

	time.Sleep(250 * time.Millisecond)
	cancelFirst()

	firstErr := <-firstDone
	if !errors.Is(firstErr, context.Canceled) {
		t.Fatalf("canceled cleanup returned err %v, want context.Canceled", firstErr)
	}

	// the interrupted lease must be voluntarily expired, so a second request
	// takes over the same generation immediately instead of being rejected
	result, err := engine.CleanLogs(context.Background(), cutoffTime, 10, false, constants.DriverSqlite)
	if err != nil {
		t.Fatalf("takeover cleanup returned err: %v", err)
	}

	if result.DeletedCount != totalLogs {
		t.Errorf("takeover cleanup counted %d, want %d (no omission or double count)", result.DeletedCount, totalLogs)
	}

	var remaining int64

	err = engine.client.Raw("SELECT COUNT(*) FROM logs").Scan(&remaining).Error
	if err != nil {
		t.Fatalf("unable to count logs: %v", err)
	}

	if remaining != 0 {
		t.Errorf("found %d remaining logs, want 0", remaining)
	}

	var (
		status     string
		generation int64
		epoch      int64
	)

	err = engine.client.Raw(
		"SELECT status, generation, epoch FROM log_cleanup_leases WHERE id = 1",
	).Row().Scan(&status, &generation, &epoch)
	if err != nil {
		t.Fatalf("unable to read lease: %v", err)
	}

	if status != cleanupStatusCompleted {
		t.Errorf("lease status = %s, want completed", status)
	}

	if generation != 1 {
		t.Errorf("generation = %d, want same generation 1", generation)
	}

	if epoch < 2 {
		t.Errorf("epoch = %d, want bumped epoch after takeover", epoch)
	}
}

// testPostgresPartitioned creates a mock postgres engine with partition-aware
// cleanup enabled.
func testPostgresPartitioned(t *testing.T) (*Engine, sqlmock.Sqlmock) {
	_sql, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Errorf("unable to create new SQL mock: %v", err)
	}

	mock.ExpectExec(CreatePostgresTable).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(CreateBuildIDIndex).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(CreateCreatedAtIndex).WillReturnResult(sqlmock.NewResult(1, 1))

	_postgres, err := testutils.TestPostgresGormInit(_sql)
	if err != nil {
		t.Errorf("unable to create new postgres database: %v", err)
	}

	_engine, err := New(
		WithClient(_postgres),
		WithLogger(logrus.NewEntry(logrus.StandardLogger())),
		WithSkipCreation(false),
		WithLogPartitioned(true),
		WithLogPartitionPattern("logs_%"),
		WithLogPartitionSchema("public"),
	)
	if err != nil {
		t.Errorf("unable to create new postgres partitioned log engine: %v", err)
	}

	return _engine, mock
}

func TestLog_Engine_CleanLogs_Partitioned(t *testing.T) {
	cutoffTime := time.Now().Add(-48 * time.Hour).Unix()

	engine, mock := testPostgresPartitioned(t)

	defer func() { _sql, _ := engine.client.DB(); _sql.Close() }()

	lease := newTestLease(cutoffTime, 100, true)

	expectLeaseTableCreate(mock)
	expectLeaseInsert(mock, lease)

	// partition discovery and snapshot
	mock.ExpectQuery(selectLogPartitionsSQL).
		WithArgs(testutils.AnyArgument{}, testutils.AnyArgument{}).
		WillReturnRows(sqlmock.NewRows([]string{"schemaname", "tablename"}).
			AddRow("public", "logs_0").
			AddRow("public", "logs_1"))
	mock.ExpectExec(pgify(startPartitionedCleanupSQL)).
		WithArgs(
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// partition 0: a full batch and then an empty batch
	expectBatch(mock, "public.logs_0", rangeIDs(1, 100), nil)
	expectBatch(mock, "public.logs_0", []int64{}, nil)
	// partition 0 checkpoint advances the partition index
	expectProgressExec(mock)

	// partition 1: a short batch finishes it
	expectBatch(mock, "public.logs_1", rangeIDs(101, 140), nil)
	expectProgressExec(mock)

	// owner-checked vacuums for each affected partition
	expectProgressExec(mock)
	mock.ExpectExec(`VACUUM ANALYZE public.logs_0`).WillReturnResult(sqlmock.NewResult(0, 0))
	expectProgressExec(mock)
	mock.ExpectExec(`VACUUM ANALYZE public.logs_1`).WillReturnResult(sqlmock.NewResult(0, 0))

	expectFinalize(mock)

	result, err := engine.CleanLogs(context.TODO(), cutoffTime, 100, true, constants.DriverPostgres)
	if err != nil {
		t.Fatalf("partitioned CleanLogs returned err: %v", err)
	}

	if result.DeletedCount != 140 {
		t.Errorf("partitioned CleanLogs returned %d deleted, want 140", result.DeletedCount)
	}

	wantPartitions := []string{"public.logs_0", "public.logs_1"}
	if !reflect.DeepEqual(result.AffectedPartitions, wantPartitions) {
		t.Errorf("partitioned CleanLogs affected = %v, want %v", result.AffectedPartitions, wantPartitions)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sql expectations: %v", err)
	}
}

func TestLog_Engine_CleanLogs_PartitionFailureFallsBackWithCumulativeStats(t *testing.T) {
	cutoffTime := time.Now().Add(-48 * time.Hour).Unix()

	engine, mock := testPostgresPartitioned(t)

	defer func() { _sql, _ := engine.client.DB(); _sql.Close() }()

	lease := newTestLease(cutoffTime, 50, true)

	expectLeaseTableCreate(mock)
	expectLeaseInsert(mock, lease)

	// discover one partition and snapshot it
	mock.ExpectQuery(selectLogPartitionsSQL).
		WithArgs(testutils.AnyArgument{}, testutils.AnyArgument{}).
		WillReturnRows(sqlmock.NewRows([]string{"schemaname", "tablename"}).
			AddRow("public", "logs_0"))
	mock.ExpectExec(pgify(startPartitionedCleanupSQL)).
		WithArgs(
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// first batch in the partition succeeds
	expectBatch(mock, "public.logs_0", rangeIDs(1, 50), nil)
	// second batch fails mid-partition; the transaction rolls back
	expectBatch(mock, "public.logs_0", nil, fmt.Errorf("partition public.logs_0 vanished"))

	// the current owner records the persisted fallback to traditional mode
	mock.ExpectExec(pgify(setCleanupModeSQL)).
		WithArgs(
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// traditional cleanup continues from cursor 0 over the parent table and
	// finds the remaining rows
	expectBatch(mock, constants.TableLog, rangeIDs(51, 53), nil)

	// vacuum preserves the partition affected before fallback, then vacuums
	// the parent table
	expectProgressExec(mock)
	mock.ExpectExec(`VACUUM ANALYZE public.logs_0`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`VACUUM ANALYZE logs`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(pgify(setVacuumDoneSQL)).
		WithArgs(
			testutils.AnyArgument{}, testutils.AnyArgument{}, testutils.AnyArgument{},
			testutils.AnyArgument{}, testutils.AnyArgument{},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	expectFinalize(mock)

	result, err := engine.CleanLogs(context.TODO(), cutoffTime, 50, true, constants.DriverPostgres)
	if err != nil {
		t.Fatalf("CleanLogs with partition failure returned err: %v", err)
	}

	// 50 deleted in the partition phase + 3 after fallback, counted once each
	if result.DeletedCount != 53 {
		t.Errorf("CleanLogs returned %d deleted, want 53", result.DeletedCount)
	}

	wantPartitions := []string{"public.logs_0"}
	if !reflect.DeepEqual(result.AffectedPartitions, wantPartitions) {
		t.Errorf("CleanLogs affected = %v, want %v", result.AffectedPartitions, wantPartitions)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sql expectations: %v", err)
	}
}
