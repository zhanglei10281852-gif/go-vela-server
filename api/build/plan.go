// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/go-vela/server/api/service"
	"github.com/go-vela/server/api/step"
	"github.com/go-vela/server/api/types"
	"github.com/go-vela/server/cache"
	"github.com/go-vela/server/compiler/types/pipeline"
	"github.com/go-vela/server/database"
	"github.com/go-vela/server/scm"
)

// PlanBuild is a helper function to plan the build for
// execution. It constructs all resources for the build -
// the build itself, all services, all steps, the log
// placeholders, and the build executable - and persists
// them as a single atomic unit.
//
// Either every resource commits together and the build is immediately
// consumable, or no resource is created and the next attempt starts without a
// conflicting build number or any orphaned child resources. All recovery is
// driven by the single database transaction's committed state, never by an
// in-process list of objects that may have been created before a failure.
func PlanBuild(ctx context.Context, db database.Interface, tknCache cache.Service, scm scm.Service, p *pipeline.Build, b *types.Build, r *types.Repo) (*types.Build, error) {
	// update fields in build object
	b.SetCreated(time.Now().UTC().Unix())

	// construct all child resources in memory; nothing is persisted yet
	services, err := service.PlanServices(p, b)
	if err != nil {
		return nil, err
	}

	steps, err := step.PlanSteps(p, b)
	if err != nil {
		return nil, err
	}

	var (
		scmRepos []string
		scmPerms map[string]string
	)

	if p.Git != nil {
		scmRepos = p.Git.Token.Repositories
		scmPerms = p.Git.Token.Permissions
	}

	// finalize runs inside the planning transaction after the build row has
	// been inserted, so the build has its final ID used to scope the SCM
	// token. It returns the marshaled executable that gets inserted in the
	// same transaction. Returning an error rolls back the entire build plan.
	finalize := func(b *types.Build, _ []*types.Service, _ []*types.Step) ([]byte, error) {
		scmBuildToken, scmBuildTokenExp, err := scm.GetNetrcPassword(ctx, db, tknCache, b, scmRepos, scmPerms)
		if err != nil {
			return nil, fmt.Errorf("failed to get SCM build token for %s: %w", r.GetFullName(), err)
		}

		p.Token = scmBuildToken
		p.TokenExp = scmBuildTokenExp

		// marshal pipeline build into byte data to add to the build executable object
		byteExecutable, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal build executable for %s: %w", r.GetFullName(), err)
		}

		return byteExecutable, nil
	}

	// atomically create the build, services, steps, logs, and executable
	result, err := db.PlanBuild(ctx, b, services, steps, finalize)
	if err != nil {
		return nil, fmt.Errorf("unable to create new build for %s: %w", r.GetFullName(), err)
	}

	b = result.Build

	logrus.WithFields(logrus.Fields{
		"build":    b.GetNumber(),
		"build_id": b.GetID(),
	}).Info("build created")

	for _, s := range result.Services {
		logrus.WithFields(logrus.Fields{
			"service":    s.GetName(),
			"service_id": s.GetID(),
			"org":        b.GetRepo().GetOrg(),
			"repo":       b.GetRepo().GetName(),
			"repo_id":    b.GetRepo().GetID(),
		}).Info("service created")
	}

	for _, s := range result.Steps {
		logrus.WithFields(logrus.Fields{
			"step":    s.GetName(),
			"step_id": s.GetID(),
			"org":     b.GetRepo().GetOrg(),
			"repo":    b.GetRepo().GetName(),
			"repo_id": b.GetRepo().GetID(),
		}).Info("step created")
	}

	// the commit status updates are external side effects that cannot be
	// rolled back, so they are only sent once the full plan is committed
	step.ReportStepStatuses(ctx, scm, p, b, result.Steps)

	return b, nil
}
