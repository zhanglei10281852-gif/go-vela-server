// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	api "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	dbtypes "github.com/go-vela/server/database/types"
)

// FinalizeBuildExecutable defines a function that is invoked within the
// planning transaction after the build, services, and steps have been inserted.
//
// The build passed to the function carries its final ID and number, so the
// returned marshaled executable data may include build-scoped values such as
// the SCM build token. Returning an error rolls back the entire planning
// transaction, leaving no build or child resources behind.
type FinalizeBuildExecutable func(b *api.Build, services []*api.Service, steps []*api.Step) ([]byte, error)

// PlanBuildResult is the set of resources created by a successful
// PlanBuild transaction.
type PlanBuildResult struct {
	// Build is the newly created build, populated with its ID and number.
	Build *api.Build
	// Services is the complete, ordered set of services created for the build.
	Services []*api.Service
	// Steps is the complete, ordered set of steps created for the build.
	Steps []*api.Step
}

// PlanBuild atomically persists the complete set of resources required for a
// build to be consumable: the build itself, every service, every step, the
// empty log placeholder for each service and step, and the build executable.
//
// All of the inserts run inside a single database transaction. If any insert
// fails, or if the finalizeExecutable callback returns an error, the
// transaction is rolled back: the repo counter is not incremented and no
// build, service, step, log, or executable rows are left behind. This
// guarantees that a retry starts from a clean persisted state, can never
// conflict with a build number from a failed attempt, and can never produce a
// second set of child resources for the same build.
//
// Recovery after a crash or partial failure is therefore driven entirely by
// the database transaction state - either every row for the build exists or
// none do - instead of any in-process list of the objects that were created.
func (e *engine) PlanBuild(ctx context.Context, b *api.Build, services []*api.Service, steps []*api.Step, finalizeExecutable FinalizeBuildExecutable) (*PlanBuildResult, error) {
	logger := e.logger.WithFields(logrus.Fields{
		"org":     b.GetRepo().GetOrg(),
		"repo":    b.GetRepo().GetName(),
		"repo_id": b.GetRepo().GetID(),
	})

	logger.Trace("planning build")

	var (
		buildID int64
		number  int64
		result  = new(PlanBuildResult)
	)

	err := e.client.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := b.GetRepo()

		if r.GetID() == 0 {
			return fmt.Errorf("repo ID must be set")
		}

		// assign the next build number by incrementing the counter for the repo
		err := tx.Raw("UPDATE repos SET counter = counter + 1 WHERE id = ? RETURNING counter", r.GetID()).Scan(&number).Error
		if err != nil {
			return err
		}

		b.SetNumber(number)

		// insert the build
		dbBuild := dbtypes.BuildFromAPI(b)

		err = dbBuild.Validate()
		if err != nil {
			return err
		}

		dbBuild = dbBuild.Crop()

		err = tx.Table(constants.TableBuild).Create(dbBuild).Error
		if err != nil {
			return err
		}

		buildID = dbBuild.ID.Int64

		b.SetID(buildID)

		// insert every service with its empty log placeholder
		plannedServices := make([]*api.Service, 0, len(services))

		for _, s := range services {
			s.SetBuildID(buildID)
			s.SetRepoID(r.GetID())

			dbService := dbtypes.ServiceFromAPI(s)

			err = dbService.Validate()
			if err != nil {
				return fmt.Errorf("unable to create service %s: %w", s.GetName(), err)
			}

			err = tx.Table(constants.TableService).Create(dbService).Error
			if err != nil {
				return fmt.Errorf("unable to create service %s: %w", s.GetName(), err)
			}

			s.SetID(dbService.ID.Int64)

			err = createPlanLog(tx, e.config.CompressionLevel, s.GetID(), 0, buildID, r.GetID())
			if err != nil {
				return fmt.Errorf("unable to create service logs for service %s: %w", s.GetName(), err)
			}

			plannedServices = append(plannedServices, s)
		}

		// insert every step with its empty log placeholder
		plannedSteps := make([]*api.Step, 0, len(steps))

		for _, s := range steps {
			s.SetBuildID(buildID)
			s.SetRepoID(r.GetID())

			dbStep := dbtypes.StepFromAPI(s)

			err = dbStep.Validate()
			if err != nil {
				return fmt.Errorf("unable to create step %s: %w", s.GetName(), err)
			}

			err = tx.Table(constants.TableStep).Create(dbStep).Error
			if err != nil {
				return fmt.Errorf("unable to create step %s: %w", s.GetName(), err)
			}

			s.SetID(dbStep.ID.Int64)

			err = createPlanLog(tx, e.config.CompressionLevel, 0, s.GetID(), buildID, r.GetID())
			if err != nil {
				return fmt.Errorf("unable to create logs for step %s: %w", s.GetName(), err)
			}

			plannedSteps = append(plannedSteps, s)
		}

		// build and insert the executable while the transaction is open so
		// the build only becomes consumable if the executable exists as well
		executableData, err := finalizeExecutable(b, plannedServices, plannedSteps)
		if err != nil {
			return err
		}

		dbExecutable := new(api.BuildExecutable)
		dbExecutable.SetBuildID(buildID)
		dbExecutable.SetData(executableData)

		executable := dbtypes.BuildExecutableFromAPI(dbExecutable)

		err = executable.Validate()
		if err != nil {
			return err
		}

		err = executable.Compress(e.config.CompressionLevel)
		if err != nil {
			return err
		}

		err = executable.Encrypt(e.config.EncryptionKey)
		if err != nil {
			return fmt.Errorf("unable to encrypt build executable for build %d: %w", buildID, err)
		}

		err = tx.Table(constants.TableBuildExecutable).Create(executable).Error
		if err != nil {
			return fmt.Errorf("unable to create build executable: %w", err)
		}

		result.Build = b
		result.Services = plannedServices
		result.Steps = plannedSteps

		return nil
	})
	if err != nil {
		// the transaction was rolled back so nothing was persisted. Reset the
		// identifiers that were assigned during planning to ensure the caller
		// does not carry a stale build number or child build IDs into a retry.
		b.SetID(0)
		b.SetNumber(0)

		for _, s := range services {
			s.SetID(0)
			s.SetBuildID(0)
		}

		for _, s := range steps {
			s.SetID(0)
			s.SetBuildID(0)
		}

		return nil, err
	}

	// keep the in-memory repo counter consistent with the database
	b.GetRepo().SetCounter(number)

	logger.WithFields(logrus.Fields{
		"build":    number,
		"build_id": buildID,
		"services": len(result.Services),
		"steps":    len(result.Steps),
	}).Info("build planned")

	return result, nil
}

// createPlanLog inserts an empty log placeholder for a newly created service
// or step using the provided transaction. Exactly one of serviceID or stepID
// must be set.
func createPlanLog(tx *gorm.DB, compressionLevel int, serviceID, stepID, buildID, repoID int64) error {
	l := new(api.Log)
	l.SetServiceID(serviceID)
	l.SetStepID(stepID)
	l.SetBuildID(buildID)
	l.SetRepoID(repoID)
	l.SetData([]byte{})
	l.SetCreatedAt(time.Now().UTC().Unix())

	dbLog := dbtypes.LogFromAPI(l)

	err := dbLog.Validate()
	if err != nil {
		return err
	}

	err = dbLog.Compress(compressionLevel)
	if err != nil {
		return err
	}

	return tx.Table(constants.TableLog).Create(dbLog).Error
}
