// SPDX-License-Identifier: Apache-2.0

package service

import (
	"time"

	"github.com/go-vela/server/api/types"
	"github.com/go-vela/server/compiler/types/pipeline"
	"github.com/go-vela/server/constants"
)

// PlanServices is a helper function to plan all services
// in the build for execution. This constructs the service
// resources for the build from the pipeline and merges the
// service environment variables into the pipeline.
//
// No database resources are created here. The returned services are
// persisted atomically, together with the build, its steps, logs, and build
// executable, by PlanBuild so that a failure while planning can never leave
// a partial set of services behind.
func PlanServices(p *pipeline.Build, b *types.Build) ([]*types.Service, error) {
	// variable to store planned services
	services := make([]*types.Service, 0, len(p.Services))

	// iterate through all pipeline services
	for _, service := range p.Services {
		// create the service object
		s := new(types.Service)
		s.SetBuildID(b.GetID())
		s.SetRepoID(b.GetRepo().GetID())
		s.SetName(service.Name)
		s.SetImage(service.Image)
		s.SetNumber(service.Number)
		s.SetStatus(constants.StatusPending)
		s.SetCreated(time.Now().UTC().Unix())

		err := service.MergeEnv(s.Environment())
		if err != nil {
			return nil, err
		}

		services = append(services, s)
	}

	return services, nil
}
