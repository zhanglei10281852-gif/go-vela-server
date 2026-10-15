// SPDX-License-Identifier: Apache-2.0

package step

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/go-vela/server/api/types"
	"github.com/go-vela/server/compiler/types/pipeline"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/scm"
)

// PlanSteps is a helper function to plan all steps
// in the build for execution. This constructs the step
// resources for the build from the pipeline and merges
// the step environment variables into the pipeline.
//
// No database resources are created here. The returned steps are persisted
// atomically, together with the build, its services, logs, and build
// executable, by PlanBuild so that a failure while planning can never leave a
// partial set of steps behind.
func PlanSteps(p *pipeline.Build, b *types.Build) ([]*types.Step, error) {
	// variable to store planned steps
	steps := []*types.Step{}

	// iterate through all pipeline stages
	for _, stage := range p.Stages {
		// iterate through all steps for each pipeline stage
		for _, c := range stage.Steps {
			s, err := planStep(b, c, stage.Name)
			if err != nil {
				return nil, err
			}

			steps = append(steps, s)
		}
	}

	// iterate through all pipeline steps
	for _, c := range p.Steps {
		s, err := planStep(b, c, "")
		if err != nil {
			return nil, err
		}

		steps = append(steps, s)
	}

	return steps, nil
}

func planStep(b *types.Build, c *pipeline.Container, stage string) (*types.Step, error) {
	// create the step object
	s := new(types.Step)
	s.SetBuildID(b.GetID())
	s.SetRepoID(b.GetRepo().GetID())
	s.SetNumber(c.Number)
	s.SetName(c.Name)
	s.SetImage(c.Image)
	s.SetStage(stage)
	s.SetStatus(constants.StatusPending)
	s.SetReportAs(c.ReportAs)
	s.SetCreated(time.Now().UTC().Unix())

	// populate environment variables from step
	err := c.MergeEnv(s.Environment())
	if err != nil {
		return nil, err
	}

	return s, nil
}

// ReportStepStatuses sends commit statuses for the steps in the build that
// were configured with report_as. This is performed after the planning
// transaction has committed because the status update is an external side
// effect that cannot be rolled back; failures are logged but do not fail the
// build, matching the previous behavior.
func ReportStepStatuses(ctx context.Context, client scm.Service, p *pipeline.Build, b *types.Build, steps []*types.Step) {
	// index the created steps by number so the pipeline containers can be
	// matched to the persisted steps carrying their final IDs
	byNumber := make(map[int32]*types.Step, len(steps))

	for _, s := range steps {
		byNumber[s.GetNumber()] = s
	}

	// collect the pipeline containers in the same order that PlanSteps does
	containers := make([]*pipeline.Container, 0, len(steps))

	for _, stage := range p.Stages {
		containers = append(containers, stage.Steps...)
	}

	containers = append(containers, p.Steps...)

	for _, c := range containers {
		if len(c.ReportAs) == 0 {
			continue
		}

		s, ok := byNumber[c.Number]
		if !ok {
			continue
		}

		// send API call to set the status on the commit using token in environment
		err := client.StepStatus(ctx, b, s, c.Environment["VELA_NETRC_PASSWORD"])
		if err != nil {
			logrus.Errorf("unable to set commit status for build: %v", err)
		}
	}
}
