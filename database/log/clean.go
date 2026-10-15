// SPDX-License-Identifier: Apache-2.0

package log

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/go-vela/server/constants"
)

// cleanupLeaseTTL is how long a cleanup ownership claim remains valid without
// a renewal. A live instance renews the lease on every batch and on a fixed
// heartbeat; if the process exits or stalls past the TTL the lease becomes
// eligible for takeover by another instance.
const cleanupLeaseTTL = 30 * time.Second

// cleanupLeaseHeartbeatInterval is how often a live owner renews its lease
// while a long running statement (or idle period) is in flight. Renewals also
// piggyback on every committed batch.
const cleanupLeaseHeartbeatInterval = cleanupLeaseTTL / 3

// cleanupLeaseAcquireMaxAttempts bounds the optimistic compare-and-swap
// retries when multiple instances race to insert or take over the lease.
const cleanupLeaseAcquireMaxAttempts = 7

// cleanupLeaseID is the primary key of the singleton lease row.
const cleanupLeaseID int64 = 1

const (
	// cleanupStatusRunning marks an unfinished cleanup whose lease may be
	// renewed, taken over, or voluntarily released.
	cleanupStatusRunning = "running"
	// cleanupStatusCompleted marks a finished cleanup. The row is retained so
	// the execution generation remains durably identifiable; the next cleanup
	// request starts a new generation against the same row.
	cleanupStatusCompleted = "completed"

	// cleanupModeTraditional is the non-partitioned deletion path.
	cleanupModeTraditional = "traditional"
	// cleanupModePartitioned is the partition-aware PostgreSQL deletion path.
	cleanupModePartitioned = "partitioned"
)

var (
	// errCleanupFenced is returned when the running instance no longer owns the
	// cleanup generation: its epoch was superseded by a takeover. Every data
	// mutation is conditional on ownership, so a fenced instance rolls the
	// current batch back, cannot vacuum, and cannot release the new owner's
	// lock.
	errCleanupFenced = errors.New("log cleanup ownership was lost: another instance took over the cleanup generation")

	// errCleanupLeaseBusy is returned while a live owner holds a valid lease.
	errCleanupLeaseBusy = errors.New("another cleanup operation is already in progress")
)

// createCleanupLeaseTable creates the singleton table that durably identifies
// each cleanup generation, its time-limited ownership, and the recoverable
// progress of the cleanup. The DDL is valid for both PostgreSQL and SQLite
// (SQLite applies INTEGER/NUMERIC affinity to BIGINT/BOOLEAN declarations).
const createCleanupLeaseTable = `
CREATE TABLE IF NOT EXISTS log_cleanup_leases (
	id                  BIGINT PRIMARY KEY,
	generation          BIGINT NOT NULL,
	epoch               BIGINT NOT NULL,
	owner_id            TEXT NOT NULL,
	leased_until        BIGINT NOT NULL,
	status              TEXT NOT NULL,
	"before"            BIGINT NOT NULL,
	batch_size          BIGINT NOT NULL,
	with_vacuum         BOOLEAN NOT NULL,
	mode                TEXT NOT NULL,
	cursor_id           BIGINT NOT NULL,
	partitions          TEXT NOT NULL,
	partition_index     BIGINT NOT NULL,
	partition_cursor_id BIGINT NOT NULL,
	affected_partitions TEXT NOT NULL,
	deleted_count       BIGINT NOT NULL,
	vacuum_done         BOOLEAN NOT NULL,
	last_error          TEXT NOT NULL,
	created_at          BIGINT NOT NULL,
	updated_at          BIGINT NOT NULL
);
`

// SQL statements used to coordinate the lease. Every mutating statement is
// conditional on (owner_id, epoch, status) so that only the current owner of
// the generation can mutate data or release the lock.
const (
	selectCleanupLeaseSQL = `
SELECT id, generation, epoch, owner_id, leased_until, status, "before", batch_size, with_vacuum, mode,
       cursor_id, partitions, partition_index, partition_cursor_id, affected_partitions,
       deleted_count, vacuum_done, last_error, created_at, updated_at
FROM log_cleanup_leases
WHERE id = ?`

	insertCleanupLeaseSQL = `
INSERT INTO log_cleanup_leases (id, generation, epoch, owner_id, leased_until, status, "before", batch_size,
                                with_vacuum, mode, cursor_id, partitions, partition_index, partition_cursor_id,
                                affected_partitions, deleted_count, vacuum_done, last_error, created_at, updated_at)
VALUES (?, 1, 1, ?, ?, 'running', ?, ?, ?, '', 0, '[]', 0, 0, '[]', 0, FALSE, '', ?, ?)
ON CONFLICT (id) DO NOTHING
RETURNING id, generation, epoch, owner_id, leased_until, status, "before", batch_size, with_vacuum, mode,
          cursor_id, partitions, partition_index, partition_cursor_id, affected_partitions,
          deleted_count, vacuum_done, last_error, created_at, updated_at`

	// newCleanupGenerationSQL resets progress and starts a new generation once
	// the previous generation reached a terminal state.
	newCleanupGenerationSQL = `
UPDATE log_cleanup_leases
SET generation = generation + 1, epoch = 1, owner_id = ?, leased_until = ?, status = 'running',
    "before" = ?, batch_size = ?, with_vacuum = ?, mode = '', cursor_id = 0, partitions = '[]',
    partition_index = 0, partition_cursor_id = 0, affected_partitions = '[]', deleted_count = 0,
    vacuum_done = FALSE, last_error = '', created_at = ?, updated_at = ?
WHERE id = ? AND status = 'completed'
RETURNING id, generation, epoch, owner_id, leased_until, status, "before", batch_size, with_vacuum, mode,
          cursor_id, partitions, partition_index, partition_cursor_id, affected_partitions,
          deleted_count, vacuum_done, last_error, created_at, updated_at`

	// takeOverCleanupLeaseSQL continues an unfinished generation. The epoch
	// (fencing token) is bumped and the compare-and-swap predicates require the
	// exact epoch and leased_until observed by the taker, so two racing takers
	// cannot both succeed.
	takeOverCleanupLeaseSQL = `
UPDATE log_cleanup_leases
SET epoch = epoch + 1, owner_id = ?, leased_until = ?, last_error = '', updated_at = ?
WHERE id = ? AND status = 'running' AND epoch = ? AND leased_until = ?
RETURNING id, generation, epoch, owner_id, leased_until, status, "before", batch_size, with_vacuum, mode,
          cursor_id, partitions, partition_index, partition_cursor_id, affected_partitions,
          deleted_count, vacuum_done, last_error, created_at, updated_at`

	renewCleanupLeaseSQL = `
UPDATE log_cleanup_leases
SET leased_until = ?, updated_at = ?
WHERE id = ? AND owner_id = ? AND epoch = ? AND status = 'running'`

	// updateCleanupProgressSQL commits batch progress and renews ownership
	// atomically with each batch's deletion. deleted_count is incremented in
	// the database so resumed attempts never double count or lose count.
	updateCleanupProgressSQL = `
UPDATE log_cleanup_leases
SET deleted_count = deleted_count + ?, cursor_id = ?, partition_index = ?, partition_cursor_id = ?,
    affected_partitions = ?, updated_at = ?, leased_until = ?
WHERE id = ? AND owner_id = ? AND epoch = ? AND status = 'running'`

	setCleanupModeSQL = `
UPDATE log_cleanup_leases
SET mode = ?, cursor_id = 0, last_error = ?, updated_at = ?, leased_until = ?
WHERE id = ? AND owner_id = ? AND epoch = ? AND status = 'running'`

	startPartitionedCleanupSQL = `
UPDATE log_cleanup_leases
SET mode = 'partitioned', partitions = ?, partition_index = 0, partition_cursor_id = 0,
    affected_partitions = '[]', last_error = '', updated_at = ?, leased_until = ?
WHERE id = ? AND owner_id = ? AND epoch = ? AND status = 'running'`

	setVacuumDoneSQL = `
UPDATE log_cleanup_leases
SET vacuum_done = TRUE, updated_at = ?, leased_until = ?
WHERE id = ? AND owner_id = ? AND epoch = ? AND status = 'running'`

	finalizeCleanupSQL = `
UPDATE log_cleanup_leases
SET status = 'completed', last_error = '', updated_at = ?, leased_until = ?
WHERE id = ? AND owner_id = ? AND epoch = ? AND status = 'running'`

	// relinquishCleanupSQL makes a lease immediately takeable by expiring it.
	// It is conditional on ownership, so a late, fenced instance cannot expire
	// the new owner's lease.
	relinquishCleanupSQL = `
UPDATE log_cleanup_leases
SET leased_until = ?, last_error = ?, updated_at = ?
WHERE id = ? AND owner_id = ? AND epoch = ? AND status = 'running'`

	// selectLogPartitionsSQL finds the partitions of the logs table.
	selectLogPartitionsSQL = `
		SELECT schemaname, tablename
		FROM pg_tables
		WHERE tablename LIKE $1
		  AND schemaname = $2
		  AND tablename != 'logs'
		ORDER BY tablename`

	// deleteCleanupBatchSQLTemplate deletes one keyset batch from a table and
	// returns the deleted ids. It is used for both the logs table and
	// individual partitions, run inside a transaction with the lease progress
	// update so deletions are ownership fenced.
	deleteCleanupBatchSQLTemplate = `DELETE FROM %s WHERE id IN (SELECT id FROM %s WHERE created_at < ? AND id > ? ORDER BY id ASC LIMIT ?) RETURNING id`
)

// CleanupResult represents the result of a log cleanup operation.
type CleanupResult struct {
	DeletedCount       int64
	AffectedPartitions []string
	// BatchSize is the batch size effectively used by the generation; on
	// takeover it is the original generation's persisted batch size.
	BatchSize int
}

// cleanupLease is the persisted state of a cleanup generation.
type cleanupLease struct {
	ID                 int64
	Generation         int64
	Epoch              int64
	OwnerID            string
	LeasedUntil        int64
	Status             string
	Before             int64
	BatchSize          int64
	WithVacuum         bool
	Mode               string
	CursorID           int64
	Partitions         string
	PartitionIndex     int64
	PartitionCursorID  int64
	AffectedPartitions string
	DeletedCount       int64
	VacuumDone         bool
	LastError          string
	CreatedAt          int64
	UpdatedAt          int64
}

// cleanupRun is the live ownership handle for a cleanup generation held by the
// current request.
type cleanupRun struct {
	ownerID    string
	generation int64
	epoch      int64

	// runCtx is canceled when the request is canceled or when the heartbeat
	// detects that ownership was fenced away.
	runCtx    context.Context
	cancelRun context.CancelFunc

	// heartbeat lifecycle
	stopHeartbeat context.CancelFunc
	heartbeatDone chan struct{}

	// fenced is set by the heartbeat once a renewal fails to find the owner's
	// epoch, which means a newer owner took over the generation.
	fenceMu sync.Mutex
	fenced  bool
}

// gated returns nil while the caller still owns the generation and the request
// is live. Otherwise it returns errCleanupFenced or the context error.
func (r *cleanupRun) gated() error {
	r.fenceMu.Lock()
	fenced := r.fenced
	r.fenceMu.Unlock()

	if fenced {
		return errCleanupFenced
	}

	select {
	case <-r.runCtx.Done():
		r.fenceMu.Lock()
		fenced = r.fenced
		r.fenceMu.Unlock()

		if fenced {
			return errCleanupFenced
		}

		return r.runCtx.Err()
	default:
	}

	return nil
}

// markFenced records that ownership was lost.
func (r *cleanupRun) markFenced() {
	r.fenceMu.Lock()
	r.fenced = true
	r.fenceMu.Unlock()
}

// CleanLogs deletes log records created before the specified timestamp in
// batches.
//
// Each invocation participates in a durable, generation-identified cleanup
// that is protected by a time-limited ownership lease:
//
//   - A live owner renews the lease per batch and via a heartbeat. If the
//     client disconnects, the owner voluntarily expires the lease; if the
//     process dies, the lease expires after the TTL.
//   - Another instance can safely take over the same unfinished generation.
//     Takeover bumps the epoch (fencing token), which fences off the late old
//     instance: deletes, partition fallback, vacuum, lock release and
//     finalization are all conditional on the current owner and epoch.
//   - Batch progress (delete count, resume cursor, partition snapshot,
//     partition index and affected partitions) is committed in the same
//     transaction as each batch, so a resumed cleanup continues from a
//     deterministic position without omissions or double counting.
//
// When partitioned mode is enabled on PostgreSQL, partition-aware cleanup is
// used; an owner-gated, persisted fallback to the traditional path is used if
// partition discovery or a partition batch fails.
func (e *Engine) CleanLogs(ctx context.Context, before int64, batchSize int, withVacuum bool, driver string) (*CleanupResult, error) {
	logrus.Tracef("cleaning logs created before %d in batches of %d", before, batchSize)

	// honor an already canceled request before touching the database
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// make sure the durable coordination table exists
	if err := e.client.WithContext(ctx).Exec(createCleanupLeaseTable).Error; err != nil {
		return nil, fmt.Errorf("failed to initialize log cleanup lease table: %w", err)
	}

	ownerID := newCleanupOwnerID()

	// acquire a fresh generation or take over an expired, unfinished one
	lease, err := e.acquireCleanupLease(ctx, ownerID, before, batchSize, withVacuum)
	if err != nil {
		return nil, err
	}

	run := &cleanupRun{
		ownerID:    ownerID,
		generation: lease.Generation,
		epoch:      lease.Epoch,
	}

	run.runCtx, run.cancelRun = context.WithCancel(ctx)

	logrus.WithFields(logrus.Fields{
		"cleanup_generation": lease.Generation,
		"cleanup_epoch":      lease.Epoch,
		"cleanup_owner":      ownerID,
		"cleanup_resumed":    lease.Epoch > 1 || lease.DeletedCount > 0,
	}).Info("acquired log cleanup lease")

	// renew the lease in the background so a long running statement cannot
	// let a live owner's lease expire
	e.startCleanupHeartbeat(run)

	// execute (or resume) the cleanup; stop the heartbeat before any release
	// or finalization so a renewal can never race the ownership transition
	result, runErr := e.runCleanup(run, lease, driver)

	run.cancelRun()
	run.stopHeartbeat()
	<-run.heartbeatDone

	if runErr == nil {
		// only the current owner may mark the generation completed
		finalized, err := e.finalizeCleanupLease(run)
		if err != nil {
			// a transient finalization failure leaves the generation running;
			// expire it so the next request resumes instead of blocking on the
			// TTL
			logrus.WithError(err).Warn("failed to finalize log cleanup lease")
			e.relinquishCleanupLease(run, err)

			return result, fmt.Errorf("failed to finalize log cleanup lease: %w", err)
		}

		if finalized {
			logrus.WithFields(logrus.Fields{
				"cleanup_generation": lease.Generation,
				"deleted_count":      lease.DeletedCount,
			}).Info("log cleanup generation completed")

			return result, nil
		}

		// ownership was fenced away after the last batch; the new owner will
		// finalize the generation
		return result, errCleanupFenced
	}

	// the cleanup did not finish: voluntarily expire the lease so another
	// instance can take over immediately (a crashed process relies on TTL)
	e.relinquishCleanupLease(run, runErr)

	return result, runErr
}

// runCleanup dispatches to the persisted cleanup mode, choosing and persisting
// the mode on the first attempt of a generation.
func (e *Engine) runCleanup(run *cleanupRun, lease *cleanupLease, driver string) (*CleanupResult, error) {
	switch lease.Mode {
	case cleanupModeTraditional:
		return e.cleanLogsTraditional(run, lease, driver)

	case cleanupModePartitioned:
		return e.cleanLogsPartitioned(run, lease)

	default:
		// first attempt of this generation: choose the path once and persist it
		if e.isPartitionedModeEnabled() && driver == constants.DriverPostgres {
			if err := e.beginPartitionedCleanup(run, lease); err == nil {
				return e.cleanLogsPartitioned(run, lease)
			} else if canceled := run.gated(); canceled != nil {
				return nil, canceled
			} else {
				logrus.WithError(err).Warn("partition-aware log cleanup unavailable; falling back to traditional cleanup")

				if err := e.switchToTraditionalMode(run, lease, err.Error()); err != nil {
					return nil, err
				}

				return e.cleanLogsTraditional(run, lease, driver)
			}
		}

		if err := e.switchToTraditionalMode(run, lease, ""); err != nil {
			return nil, err
		}

		return e.cleanLogsTraditional(run, lease, driver)
	}
}

// cleanLogsTraditional deletes logs in keyset batches from the logs table. It
// resumes from the persisted cursor and accumulates the persisted delete
// count, so partial failures are retried without omissions or double counts.
func (e *Engine) cleanLogsTraditional(run *cleanupRun, lease *cleanupLease, driver string) (*CleanupResult, error) {
	apply := func(l *cleanupLease, ids []int64) {
		if len(ids) > 0 {
			l.CursorID = maxCleanupID(ids)
		}
	}

	for {
		if err := run.gated(); err != nil {
			return traditionalCleanupResult(lease), err
		}

		n, err := e.deleteCleanupBatch(run, lease, constants.TableLog, lease.CursorID, apply)
		if err != nil {
			return traditionalCleanupResult(lease), fmt.Errorf("failed to delete batch of logs: %w", err)
		}

		// a short batch (or an empty one) means no matching rows remain
		if n == 0 || n < lease.BatchSize {
			break
		}

		if err := cleanupBatchSleep(run); err != nil {
			return traditionalCleanupResult(lease), err
		}
	}

	logrus.Infof("cleaned %d logs created before %d", lease.DeletedCount, lease.Before)

	if err := e.vacuumTraditionalCleanup(run, lease, driver); err != nil {
		return traditionalCleanupResult(lease), err
	}

	return traditionalCleanupResult(lease), nil
}

// cleanLogsPartitioned implements partition-aware cleanup for PostgreSQL
// partitioned tables. The partition list is snapshotted once and durably
// resumed by index; per-partition cursors and the affected partition list are
// committed with each batch.
func (e *Engine) cleanLogsPartitioned(run *cleanupRun, lease *cleanupLease) (*CleanupResult, error) {
	partitions := decodeCleanupList(lease.Partitions)
	affected := decodeCleanupList(lease.AffectedPartitions)
	affectedSet := make(map[string]struct{}, len(affected))

	for _, partition := range affected {
		affectedSet[partition] = struct{}{}
	}

	fallbackToTraditional := func(cause error) (*CleanupResult, error) {
		logrus.WithError(cause).Warn("partition-aware log cleanup failed; falling back to traditional cleanup")

		if err := e.switchToTraditionalMode(run, lease, cause.Error()); err != nil {
			return partitionedCleanupResult(lease, affected), err
		}

		// continue as the current owner with the already accumulated count and
		// affected partition list intact
		return e.cleanLogsTraditional(run, lease, constants.DriverPostgres)
	}

	for idx := int(lease.PartitionIndex); idx < len(partitions); idx++ {
		partition := partitions[idx]

		apply := func(l *cleanupLease, ids []int64) {
			if len(ids) == 0 {
				return
			}

			l.PartitionCursorID = maxCleanupID(ids)

			if _, ok := affectedSet[partition]; !ok {
				affectedSet[partition] = struct{}{}
				affected = append(affected, partition)
				l.AffectedPartitions = encodeCleanupList(affected)
			}
		}

		for {
			if err := run.gated(); err != nil {
				return partitionedCleanupResult(lease, affected), err
			}

			n, err := e.deleteCleanupBatch(run, lease, partition, lease.PartitionCursorID, apply)
			if err != nil {
				if errors.Is(err, errCleanupFenced) || isContextDoneError(err) {
					return partitionedCleanupResult(lease, affected), err
				}

				return fallbackToTraditional(fmt.Errorf("failed to clean partition %s: %w", partition, err))
			}

			if n == 0 || n < lease.BatchSize {
				// partition is complete; durably advance the resume position
				lease.PartitionIndex = int64(idx + 1)
				lease.PartitionCursorID = 0

				if err := e.checkpointCleanupProgress(run, lease); err != nil {
					return partitionedCleanupResult(lease, affected), err
				}

				break
			}

			if err := cleanupBatchSleep(run); err != nil {
				return partitionedCleanupResult(lease, affected), err
			}
		}
	}

	logrus.Infof("partition-aware cleanup deleted %d logs from %d partitions", lease.DeletedCount, len(affected))

	// vacuum only the partitions touched by this generation
	if lease.WithVacuum && len(affected) > 0 {
		if err := run.gated(); err != nil {
			return partitionedCleanupResult(lease, affected), err
		}

		for _, partition := range affected {
			if err := e.vacuumAsOwner(run, lease, constants.DriverPostgres, partition); err != nil {
				return partitionedCleanupResult(lease, affected), err
			}
		}
	}

	return partitionedCleanupResult(lease, affected), nil
}

// beginPartitionedCleanup discovers the partition set and durably snapshots it
// for this generation before any deletion.
func (e *Engine) beginPartitionedCleanup(run *cleanupRun, lease *cleanupLease) error {
	partitions, err := e.discoverLogPartitions(run.runCtx)
	if err != nil {
		return fmt.Errorf("failed to discover log partitions: %w", err)
	}

	if len(partitions) == 0 {
		return fmt.Errorf("no log partitions found matching pattern %s", e.config.LogPartitionPattern)
	}

	logrus.Debugf("snapshotting %d log partitions for cleanup generation %d", len(partitions), lease.Generation)

	snapshot := encodeCleanupList(partitions)

	now := time.Now().Unix()

	result := e.client.WithContext(run.runCtx).Exec(
		startPartitionedCleanupSQL,
		snapshot, now, now+int64(cleanupLeaseTTL/time.Second), cleanupLeaseID, run.ownerID, run.epoch,
	)
	if result.Error != nil {
		return fmt.Errorf("failed to persist partitioned cleanup mode: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errCleanupFenced
	}

	lease.Mode = cleanupModePartitioned
	lease.Partitions = snapshot
	lease.PartitionIndex = 0
	lease.PartitionCursorID = 0
	lease.AffectedPartitions = "[]"

	return nil
}

// switchToTraditionalMode durably records the owner-gated fallback from
// partition-aware cleanup to traditional cleanup. The accumulated delete count
// and affected partition list are preserved across the fallback.
func (e *Engine) switchToTraditionalMode(run *cleanupRun, lease *cleanupLease, cause string) error {
	now := time.Now().Unix()

	result := e.client.WithContext(run.runCtx).Exec(
		setCleanupModeSQL,
		cleanupModeTraditional, cause, now, now+int64(cleanupLeaseTTL/time.Second), cleanupLeaseID, run.ownerID, run.epoch,
	)
	if result.Error != nil {
		return fmt.Errorf("failed to persist traditional cleanup mode fallback: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errCleanupFenced
	}

	lease.Mode = cleanupModeTraditional
	lease.CursorID = 0
	lease.LastError = cause

	return nil
}

// deleteCleanupBatch deletes one keyset batch from the target table and
// commits the deletion together with the progress/ownership update in a single
// transaction.
//
// If the owner no longer owns the generation, the progress update affects no
// rows and the whole transaction (including the deletion) rolls back, so a
// fenced instance cannot modify data and the new owner redoes the exact batch
// with correct accounting.
func (e *Engine) deleteCleanupBatch(run *cleanupRun, lease *cleanupLease, table string, cursor int64, apply func(*cleanupLease, []int64)) (int64, error) {
	query := fmt.Sprintf(deleteCleanupBatchSQLTemplate, table, table)

	var deleted int64

	err := e.client.WithContext(run.runCtx).Transaction(func(tx *gorm.DB) error {
		rows, err := tx.Raw(query, lease.Before, cursor, lease.BatchSize).Rows()
		if err != nil {
			return err
		}

		ids := make([]int64, 0, lease.BatchSize)

		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}

			ids = append(ids, id)
		}

		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}

		_ = rows.Close()

		deleted = int64(len(ids))

		// apply batch specific progress (resume cursor, affected partitions)
		// so it is persisted in this same transaction
		if apply != nil {
			apply(lease, ids)
		}

		result := e.execCleanupProgress(tx, run, lease, deleted)
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			// ownership was fenced away; roll the deletion back with the
			// progress update
			return errCleanupFenced
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	// the deletion and the durable increment committed together, so mirror the
	// increment in memory only now
	if deleted > 0 {
		lease.DeletedCount += deleted
	}

	logrus.Debugf("deleted batch of %d logs from %s (generation %d total: %d)", deleted, table, lease.Generation, lease.DeletedCount)

	return deleted, nil
}

// execCleanupProgress writes the current progress and renews the lease. It
// works on a transaction or on the pooled client because both are *gorm.DB.
func (e *Engine) execCleanupProgress(db *gorm.DB, run *cleanupRun, lease *cleanupLease, addDeleted int64) *gorm.DB {
	now := time.Now().Unix()

	return db.Exec(
		updateCleanupProgressSQL,
		addDeleted,
		lease.CursorID,
		lease.PartitionIndex,
		lease.PartitionCursorID,
		lease.AffectedPartitions,
		now,
		now+int64(cleanupLeaseTTL/time.Second),
		cleanupLeaseID,
		run.ownerID,
		run.epoch,
	)
}

// checkpointCleanupProgress persists the current progress without deleting
// anything and doubles as an ownership fence check between batches.
func (e *Engine) checkpointCleanupProgress(run *cleanupRun, lease *cleanupLease) error {
	result := e.execCleanupProgress(e.client.WithContext(run.runCtx), run, lease, 0)
	if result.Error != nil {
		return fmt.Errorf("failed to checkpoint log cleanup progress: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errCleanupFenced
	}

	return nil
}

// vacuumTraditionalCleanup runs the best-effort post-cleanup vacuum for the
// traditional path. On PostgreSQL it also vacuums partitions affected before
// a persisted partition-to-traditional fallback.
func (e *Engine) vacuumTraditionalCleanup(run *cleanupRun, lease *cleanupLease, driver string) error {
	if !lease.WithVacuum || lease.DeletedCount == 0 || lease.VacuumDone {
		return nil
	}

	// re-verify ownership immediately before vacuuming
	if err := e.checkpointCleanupProgress(run, lease); err != nil {
		return err
	}

	vacuumFailed := false

	for _, partition := range decodeCleanupList(lease.AffectedPartitions) {
		if err := e.vacuumTable(run.runCtx, driver, partition); err != nil {
			// vacuum failures remain best effort and do not fail the cleanup
			vacuumFailed = true

			logrus.WithError(err).Warnf("failed to vacuum log partition %s after cleanup", partition)
		} else {
			logrus.Debugf("successfully vacuumed log partition %s after cleanup", partition)
		}
	}

	if err := e.vacuumTable(run.runCtx, driver, ""); err != nil {
		vacuumFailed = true

		logrus.WithError(err).Warn("failed to vacuum logs table after cleanup")
	} else {
		logrus.Info("successfully vacuumed logs table after cleanup")
	}

	// leave vacuum_done unset when any vacuum failed so a takeover retries it
	if vacuumFailed {
		return nil
	}

	now := time.Now().Unix()

	result := e.client.WithContext(run.runCtx).Exec(
		setVacuumDoneSQL,
		now, now+int64(cleanupLeaseTTL/time.Second), cleanupLeaseID, run.ownerID, run.epoch,
	)
	if result.Error != nil {
		return fmt.Errorf("failed to persist vacuum state: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errCleanupFenced
	}

	lease.VacuumDone = true

	return nil
}

// vacuumAsOwner verifies ownership and vacuums a single partition. VACUUM can
// not run inside a transaction block, so the ownership check and the vacuum are
// separate, owner-checked, sequential statements.
func (e *Engine) vacuumAsOwner(run *cleanupRun, lease *cleanupLease, driver, partition string) error {
	if err := e.checkpointCleanupProgress(run, lease); err != nil {
		return err
	}

	if err := e.vacuumTable(run.runCtx, driver, partition); err != nil {
		// best effort, matching the historical partition vacuum behavior
		logrus.WithError(err).Warnf("failed to vacuum log partition %s", partition)
		return nil
	}

	logrus.Debugf("successfully vacuumed log partition %s", partition)

	return nil
}

// vacuumTable runs VACUUM for the whole logs table (table == "") or for a
// specific partition.
func (e *Engine) vacuumTable(ctx context.Context, driver, table string) error {
	if len(table) > 0 {
		// partition-aware PostgreSQL only
		return e.client.WithContext(ctx).Exec(fmt.Sprintf("VACUUM ANALYZE %s", table)).Error
	}

	switch driver {
	case constants.DriverPostgres:
		return e.client.WithContext(ctx).Exec("VACUUM ANALYZE logs").Error

	case constants.DriverSqlite:
		fallthrough
	default:
		// SQLite VACUUM cannot run inside a transaction
		return e.client.WithContext(ctx).Exec("VACUUM").Error
	}
}

// acquireCleanupLease inserts the first generation, starts a new generation
// after completion, rejects a request while a valid lease is held, or takes
// over an expired, unfinished generation.
func (e *Engine) acquireCleanupLease(ctx context.Context, ownerID string, before int64, batchSize int, withVacuum bool) (*cleanupLease, error) {
	var lastErr error

	for attempt := 0; attempt < cleanupLeaseAcquireMaxAttempts; attempt++ {
		if attempt > 0 {
			// brief backoff while racing another acquirer
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*20) * time.Millisecond):
			}
		}

		lease, found, err := e.readCleanupLease(ctx)
		if err != nil {
			return nil, err
		}

		now := time.Now().Unix()
		leasedUntil := now + int64(cleanupLeaseTTL/time.Second)

		if !found {
			acquired, got, err := e.acquireLeaseWithReturning(ctx, insertCleanupLeaseSQL,
				cleanupLeaseID, ownerID, leasedUntil, before, int64(batchSize), withVacuum, now, now)
			if err != nil {
				return nil, fmt.Errorf("failed to create log cleanup lease: %w", err)
			}

			if !got {
				// raced with a concurrent insert
				continue
			}

			return acquired, nil
		}

		switch lease.Status {
		case cleanupStatusCompleted:
			acquired, got, err := e.acquireLeaseWithReturning(ctx, newCleanupGenerationSQL,
				ownerID, leasedUntil, before, int64(batchSize), withVacuum, now, now, cleanupLeaseID)
			if err != nil {
				return nil, fmt.Errorf("failed to start new log cleanup generation: %w", err)
			}

			if !got {
				// the completed generation was started by someone else
				continue
			}

			return acquired, nil

		case cleanupStatusRunning:
			if lease.LeasedUntil > now {
				// a live owner still holds a valid lease
				logrus.WithFields(logrus.Fields{
					"cleanup_generation": lease.Generation,
					"cleanup_owner":      lease.OwnerID,
				}).Debug("log cleanup lease is already held by a live owner")

				return nil, errCleanupLeaseBusy
			}

			// lease expired: take over the unfinished generation by bumping the
			// epoch, fencing the late old owner
			acquired, got, err := e.acquireLeaseWithReturning(ctx, takeOverCleanupLeaseSQL,
				ownerID, leasedUntil, now, cleanupLeaseID, lease.Epoch, lease.LeasedUntil)
			if err != nil {
				return nil, fmt.Errorf("failed to take over log cleanup lease: %w", err)
			}

			if !got {
				// another taker won the race; re-read and either reject or retry
				lastErr = errCleanupLeaseBusy
				continue
			}

			logrus.WithFields(logrus.Fields{
				"cleanup_generation": acquired.Generation,
				"cleanup_epoch":      acquired.Epoch,
				"recovered_deleted":  acquired.DeletedCount,
			}).Info("took over expired log cleanup generation")

			return acquired, nil

		default:
			return nil, fmt.Errorf("unknown log cleanup lease status %q", lease.Status)
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("unable to acquire log cleanup lease after %d attempts", cleanupLeaseAcquireMaxAttempts)
}

// acquireLeaseWithReturning runs an INSERT/...RETURNING or UPDATE/...RETURNING
// acquisition statement and scans the acquired lease. A zero-row result means
// the compare-and-swap failed and another acquirer won.
func (e *Engine) acquireLeaseWithReturning(ctx context.Context, query string, args ...interface{}) (*cleanupLease, bool, error) {
	rows, err := e.client.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, false, rows.Err()
	}

	lease, err := scanCleanupLease(rows)
	if err != nil {
		return nil, false, err
	}

	return lease, true, rows.Err()
}

// rowScanner is satisfied by both *sql.Rows and gorm row wrappers.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanCleanupLease scans one lease row in column order.
func scanCleanupLease(rows rowScanner) (*cleanupLease, error) {
	lease := new(cleanupLease)

	err := rows.Scan(
		&lease.ID,
		&lease.Generation,
		&lease.Epoch,
		&lease.OwnerID,
		&lease.LeasedUntil,
		&lease.Status,
		&lease.Before,
		&lease.BatchSize,
		&lease.WithVacuum,
		&lease.Mode,
		&lease.CursorID,
		&lease.Partitions,
		&lease.PartitionIndex,
		&lease.PartitionCursorID,
		&lease.AffectedPartitions,
		&lease.DeletedCount,
		&lease.VacuumDone,
		&lease.LastError,
		&lease.CreatedAt,
		&lease.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan log cleanup lease: %w", err)
	}

	return lease, nil
}

// readCleanupLease reads the singleton lease row, reporting found=false when
// no generation exists yet.
func (e *Engine) readCleanupLease(ctx context.Context) (*cleanupLease, bool, error) {
	rows, err := e.client.WithContext(ctx).Raw(selectCleanupLeaseSQL, cleanupLeaseID).Rows()
	if err != nil {
		return nil, false, fmt.Errorf("failed to read log cleanup lease: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, false, rows.Err()
	}

	lease, err := scanCleanupLease(rows)
	if err != nil {
		return nil, false, err
	}

	return lease, true, rows.Err()
}

// startCleanupHeartbeat renews the lease until stopped. A failed renewal
// (owner epoch no longer present) fences the run and cancels its context so
// in-flight work stops promptly.
func (e *Engine) startCleanupHeartbeat(run *cleanupRun) {
	hbCtx, stop := context.WithCancel(context.Background())
	run.stopHeartbeat = stop
	run.heartbeatDone = make(chan struct{})

	go func() {
		defer close(run.heartbeatDone)

		ticker := time.NewTicker(cleanupLeaseHeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				now := time.Now().Unix()

				result := e.client.WithContext(hbCtx).Exec(
					renewCleanupLeaseSQL,
					now+int64(cleanupLeaseTTL/time.Second), now, cleanupLeaseID, run.ownerID, run.epoch,
				)
				if result.Error != nil {
					// transient errors are retried on the next tick; if the
					// lease actually expires, batch updates fail closed
					logrus.WithError(result.Error).Debug("failed to renew log cleanup lease")
					continue
				}

				if result.RowsAffected == 0 {
					logrus.WithFields(logrus.Fields{
						"cleanup_generation": run.generation,
						"cleanup_owner":      run.ownerID,
					}).Warn("log cleanup lease was fenced by another owner; stopping stale cleanup work")

					run.markFenced()
					run.cancelRun()

					return
				}
			}
		}
	}()
}

// finalizeCleanupLease marks the generation completed. It reports whether the
// current owner performed the finalization.
func (e *Engine) finalizeCleanupLease(run *cleanupRun) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().Unix()

	result := e.client.WithContext(ctx).Exec(
		finalizeCleanupSQL,
		now, now+int64(cleanupLeaseTTL/time.Second), cleanupLeaseID, run.ownerID, run.epoch,
	)
	if result.Error != nil {
		return false, result.Error
	}

	return result.RowsAffected == 1, nil
}

// relinquishCleanupLease voluntarily expires an unfinished lease so another
// instance can take over immediately. A no-op occurs when the caller is no
// longer the owner, ensuring a stale instance cannot release a new owner's
// lock.
func (e *Engine) relinquishCleanupLease(run *cleanupRun, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().Unix()
	lastError := ""

	if cause != nil {
		lastError = cause.Error()
		if len(lastError) > 1024 {
			lastError = lastError[:1024]
		}
	}

	result := e.client.WithContext(ctx).Exec(
		relinquishCleanupSQL,
		now-1, lastError, now, cleanupLeaseID, run.ownerID, run.epoch,
	)
	if result.Error != nil {
		logrus.WithError(result.Error).Warn("failed to relinquish log cleanup lease")
		return
	}

	if result.RowsAffected == 1 {
		logrus.WithField("cleanup_generation", run.generation).Debug("relinquished log cleanup lease for takeover")
	} else {
		logrus.WithField("cleanup_generation", run.generation).Debug("skipped relinquish: no longer the cleanup owner")
	}
}

// newCleanupOwnerID builds a unique, debuggable owner identity for one request.
func newCleanupOwnerID() string {
	host, err := os.Hostname()
	if err != nil || len(host) == 0 {
		host = "unknown"
	}

	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), uuid.NewString())
}

// cleanupBatchSleep pauses between batches while honoring cancellation and
// fence loss.
func cleanupBatchSleep(run *cleanupRun) error {
	select {
	case <-run.runCtx.Done():
		run.fenceMu.Lock()
		fenced := run.fenced
		run.fenceMu.Unlock()

		if fenced {
			return errCleanupFenced
		}

		return run.runCtx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// isContextDoneError reports whether err is a context cancellation or timeout.
func isContextDoneError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// maxCleanupID returns the largest id in a non-empty batch.
func maxCleanupID(ids []int64) int64 {
	max := ids[0]

	for _, id := range ids[1:] {
		if id > max {
			max = id
		}
	}

	return max
}

// encodeCleanupList JSON encodes a string list for durable progress storage.
func encodeCleanupList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}

	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}

	return string(data)
}

// decodeCleanupList JSON decodes a durable string list, always returning a
// non-nil slice.
func decodeCleanupList(raw string) []string {
	if len(raw) == 0 {
		return []string{}
	}

	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}

	if items == nil {
		return []string{}
	}

	return items
}

// traditionalCleanupResult builds a result for the traditional path,
// preserving partitions affected before a persisted fallback.
func traditionalCleanupResult(lease *cleanupLease) *CleanupResult {
	return &CleanupResult{
		DeletedCount:       lease.DeletedCount,
		AffectedPartitions: decodeCleanupList(lease.AffectedPartitions),
		BatchSize:          int(lease.BatchSize),
	}
}

// partitionedCleanupResult builds a result for the partition-aware path.
func partitionedCleanupResult(lease *cleanupLease, affected []string) *CleanupResult {
	return &CleanupResult{
		DeletedCount:       lease.DeletedCount,
		AffectedPartitions: affected,
		BatchSize:          int(lease.BatchSize),
	}
}

// isPartitionedModeEnabled checks if partition-aware cleanup is enabled.
func (e *Engine) isPartitionedModeEnabled() bool {
	return e.config.LogPartitioned
}

// discoverLogPartitions queries the PostgreSQL system catalogs to find log table partitions.
func (e *Engine) discoverLogPartitions(ctx context.Context) ([]string, error) {
	var partitions []string

	rows, err := e.client.WithContext(ctx).Raw(selectLogPartitionsSQL, e.config.LogPartitionPattern, e.config.LogPartitionSchema).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to query log partitions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var schema, table string
		if err := rows.Scan(&schema, &table); err != nil {
			return nil, fmt.Errorf("failed to scan partition row: %w", err)
		}

		// use schema.table format for qualified table names
		partitionName := fmt.Sprintf("%s.%s", schema, table)
		partitions = append(partitions, partitionName)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating partition rows: %w", err)
	}

	return partitions, nil
}
