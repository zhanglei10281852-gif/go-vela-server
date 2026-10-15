// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	api "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/tracing"
)

// testPlanEngine creates an isolated in-memory sqlite engine for planning tests.
func testPlanEngine(t *testing.T) Interface {
	t.Helper()

	// use a unique named in-memory database per test so shared-cache databases
	// do not leak state between tests
	name := sanitizeDSNName(t.Name())

	db, err := New(
		WithAddress(fmt.Sprintf("file:planbuild_%s?mode=memory&cache=shared", name)),
		WithCompressionLevel(3),
		WithConnectionLife(30*time.Minute),
		WithConnectionIdle(2),
		WithConnectionOpen(0),
		WithDriver("sqlite3"),
		WithEncryptionKey("A1B2C3D4E5G6H7I8J9K0LMNOPQRSTUVW"),
		WithSkipCreation(false),
		WithLogLevel("warn"),
		WithLogShowSQL(false),
		WithLogSkipNotFound(true),
		WithLogSlowThreshold(200*time.Millisecond),
		WithTracing(&tracing.Client{Config: tracing.Config{EnableTracing: false}}),
	)
	if err != nil {
		t.Fatalf("unable to create test database engine: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db
}

func sanitizeDSNName(name string) string {
	out := make([]byte, 0, len(name))

	for i := range len(name) {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}

	return string(out)
}

// testPlanRepo creates a user and repo that can own planned builds.
func testPlanRepo(t *testing.T, db Interface) *api.Repo {
	t.Helper()

	ctx := context.Background()

	u := new(api.User)
	u.SetName("octocat")
	u.SetToken("superSecretToken")
	u.SetRefreshToken("superSecretRefreshToken")
	u.SetActive(true)
	u.SetAdmin(false)

	u, err := db.CreateUser(ctx, u)
	if err != nil {
		t.Fatalf("unable to create test user: %v", err)
	}

	r := new(api.Repo)
	r.SetOwner(u)
	r.SetHash("MzM4N2MzMDAtNmY4Mi00OTA5LWFhZDAtNWIzMTlkNTJkODMy")
	r.SetOrg("github")
	r.SetName("octocat")
	r.SetFullName("github/octocat")
	r.SetLink("https://github.com/github/octocat")
	r.SetClone("https://github.com/github/octocat.git")
	r.SetBranch("main")
	r.SetBuildLimit(10)
	r.SetTimeout(30)
	r.SetCounter(0)
	r.SetHookCounter(0)
	r.SetVisibility("public")
	r.SetActive(true)
	r.SetPipelineType("")
	r.SetApproveBuild(constants.ApproveNever)
	r.SetAllowEvents(api.NewEventsFromMask(1))
	r.SetMergeQueueEvents([]string{})
	r.SetApprovalTimeout(7)

	r, err = db.CreateRepo(ctx, r)
	if err != nil {
		t.Fatalf("unable to create test repo: %v", err)
	}

	return r
}

func testPlanBuild(r *api.Repo) *api.Build {
	b := new(api.Build)
	b.SetRepo(r)
	b.SetEvent(constants.EventPush)
	b.SetStatus(constants.StatusPending)
	b.SetCommit("48afb5bdc41ad69bf22588491333f7cf71135163")
	b.SetBranch("main")
	b.SetRef("refs/heads/main")
	b.SetCreated(time.Now().UTC().Unix())

	return b
}

func testPlanServices(b *api.Build) []*api.Service {
	services := make([]*api.Service, 2)

	for i := range services {
		s := new(api.Service)
		s.SetBuildID(b.GetID())
		s.SetRepoID(b.GetRepo().GetID())
		s.SetNumber(int32(i + 1))
		s.SetName(fmt.Sprintf("service_%d", i+1))
		s.SetImage("alpine:latest")
		s.SetStatus(constants.StatusPending)
		s.SetCreated(time.Now().UTC().Unix())

		services[i] = s
	}

	return services
}

func testPlanSteps(b *api.Build) []*api.Step {
	steps := make([]*api.Step, 3)

	for i := range steps {
		s := new(api.Step)
		s.SetBuildID(b.GetID())
		s.SetRepoID(b.GetRepo().GetID())
		s.SetNumber(int32(i + 1))
		s.SetName(fmt.Sprintf("step_%d", i+1))
		s.SetImage("alpine:latest")
		s.SetStatus(constants.StatusPending)
		s.SetCreated(time.Now().UTC().Unix())

		steps[i] = s
	}

	return steps
}

// finalizeExecutable is the default successful finalizer used by the tests.
func finalizeExecutable(data []byte, calls *int) FinalizeBuildExecutable {
	return func(b *api.Build, _ []*api.Service, _ []*api.Step) ([]byte, error) {
		if calls != nil {
			*calls++
		}

		// the finalizer must run once the build has been assigned its identity
		if b.GetID() == 0 || b.GetNumber() == 0 {
			return nil, errors.New("finalizer invoked without a persisted build identity")
		}

		return data, nil
	}
}

func TestEngine_PlanBuild_Success(t *testing.T) {
	db := testPlanEngine(t)
	r := testPlanRepo(t, db)
	b := testPlanBuild(r)
	services := testPlanServices(b)
	steps := testPlanSteps(b)

	executableData := []byte(`{"id":"plan-build-success"}`)

	result, err := db.PlanBuild(context.Background(), b, services, steps, finalizeExecutable(executableData, nil))
	if err != nil {
		t.Fatalf("PlanBuild returned err: %v", err)
	}

	if result.Build.GetNumber() != 1 {
		t.Errorf("build number = %d, want 1", result.Build.GetNumber())
	}

	if result.Build.GetID() == 0 {
		t.Error("build ID should be populated")
	}

	if len(result.Services) != len(services) {
		t.Errorf("planned services = %d, want %d", len(result.Services), len(services))
	}

	if len(result.Steps) != len(steps) {
		t.Errorf("planned steps = %d, want %d", len(result.Steps), len(steps))
	}

	for _, s := range result.Services {
		if s.GetID() == 0 {
			t.Errorf("service %s should have an ID", s.GetName())
		}

		if s.GetBuildID() != result.Build.GetID() {
			t.Errorf("service %s build_id = %d, want %d", s.GetName(), s.GetBuildID(), result.Build.GetID())
		}
	}

	for _, s := range result.Steps {
		if s.GetID() == 0 {
			t.Errorf("step %s should have an ID", s.GetName())
		}

		if s.GetBuildID() != result.Build.GetID() {
			t.Errorf("step %s build_id = %d, want %d", s.GetName(), s.GetBuildID(), result.Build.GetID())
		}
	}

	// exactly one build should exist for the repo
	count, err := db.CountBuilds(context.Background())
	if err != nil {
		t.Fatalf("CountBuilds returned err: %v", err)
	}

	if count != 1 {
		t.Errorf("CountBuilds = %d, want 1", count)
	}

	// the repo counter should reflect the assigned number
	gotRepo, err := db.GetRepo(context.Background(), r.GetID())
	if err != nil {
		t.Fatalf("GetRepo returned err: %v", err)
	}

	if gotRepo.GetCounter() != 1 {
		t.Errorf("repo counter = %d, want 1", gotRepo.GetCounter())
	}

	// the full set of services, steps, and their logs should be readable
	gotServices, err := db.ListServicesForBuild(context.Background(), result.Build, nil, 1, 100)
	if err != nil {
		t.Fatalf("ListServicesForBuild returned err: %v", err)
	}

	if len(gotServices) != len(services) {
		t.Errorf("persisted services = %d, want %d", len(gotServices), len(services))
	}

	gotSteps, err := db.ListStepsForBuild(context.Background(), result.Build, nil, 1, 100)
	if err != nil {
		t.Fatalf("ListStepsForBuild returned err: %v", err)
	}

	if len(gotSteps) != len(steps) {
		t.Errorf("persisted steps = %d, want %d", len(gotSteps), len(steps))
	}

	gotLogs, err := db.ListLogsForBuild(context.Background(), result.Build, 1, 100)
	if err != nil {
		t.Fatalf("ListLogsForBuild returned err: %v", err)
	}

	if len(gotLogs) != len(services)+len(steps) {
		t.Errorf("persisted logs = %d, want %d", len(gotLogs), len(services)+len(steps))
	}

	// the executable should be present and round-trip through the pop path,
	// which decrypts and decompresses the data
	gotExecutable, err := db.PopBuildExecutable(context.Background(), result.Build.GetID())
	if err != nil {
		t.Fatalf("PopBuildExecutable returned err: %v", err)
	}

	if string(gotExecutable.GetData()) != string(executableData) {
		t.Errorf("executable data = %s, want %s", string(gotExecutable.GetData()), string(executableData))
	}
}

func TestEngine_PlanBuild_FinalizeErrorRollsBack(t *testing.T) {
	db := testPlanEngine(t)
	r := testPlanRepo(t, db)
	b := testPlanBuild(r)
	services := testPlanServices(b)
	steps := testPlanSteps(b)

	finalizeCalls := 0

	failingFinalize := func(_ *api.Build, _ []*api.Service, _ []*api.Step) ([]byte, error) {
		finalizeCalls++

		return nil, errors.New("unable to marshal build executable")
	}

	result, err := db.PlanBuild(context.Background(), b, services, steps, failingFinalize)
	if err == nil {
		t.Fatal("PlanBuild should have returned err for failing finalizer")
	}

	if result != nil {
		t.Fatal("PlanBuild should not return a result on failure")
	}

	if finalizeCalls != 1 {
		t.Errorf("failing finalizer calls = %d, want 1", finalizeCalls)
	}

	// nothing should have been committed
	count, err := db.CountBuilds(context.Background())
	if err != nil {
		t.Fatalf("CountBuilds returned err: %v", err)
	}

	if count != 0 {
		t.Errorf("CountBuilds after rollback = %d, want 0", count)
	}

	gotRepo, err := db.GetRepo(context.Background(), r.GetID())
	if err != nil {
		t.Fatalf("GetRepo returned err: %v", err)
	}

	if gotRepo.GetCounter() != 0 {
		t.Errorf("repo counter after rollback = %d, want 0", gotRepo.GetCounter())
	}

	// identifiers assigned in memory must be reset so the caller can retry
	if b.GetID() != 0 || b.GetNumber() != 0 {
		t.Errorf("build identity after rollback = (%d,%d), want (0,0)", b.GetID(), b.GetNumber())
	}

	for _, s := range services {
		if s.GetID() != 0 || s.GetBuildID() != 0 {
			t.Errorf("service %s identity after rollback = (%d,%d), want (0,0)", s.GetName(), s.GetID(), s.GetBuildID())
		}
	}

	for _, s := range steps {
		if s.GetID() != 0 || s.GetBuildID() != 0 {
			t.Errorf("step %s identity after rollback = (%d,%d), want (0,0)", s.GetName(), s.GetID(), s.GetBuildID())
		}
	}

	// retrying with fresh child resources must converge on a single build with
	// a single set of children and a single executable
	b = testPlanBuild(r)

	retryServices := testPlanServices(b)
	retrySteps := testPlanSteps(b)

	executableData := []byte(`{"id":"plan-build-retry"}`)

	retryResult, err := db.PlanBuild(context.Background(), b, retryServices, retrySteps, finalizeExecutable(executableData, nil))
	if err != nil {
		t.Fatalf("retry PlanBuild returned err: %v", err)
	}

	if retryResult.Build.GetNumber() != 1 {
		t.Errorf("retry build number = %d, want 1", retryResult.Build.GetNumber())
	}

	count, err = db.CountBuilds(context.Background())
	if err != nil {
		t.Fatalf("CountBuilds after retry returned err: %v", err)
	}

	if count != 1 {
		t.Errorf("CountBuilds after retry = %d, want 1", count)
	}

	gotServices, err := db.ListServicesForBuild(context.Background(), retryResult.Build, nil, 1, 100)
	if err != nil {
		t.Fatalf("ListServicesForBuild returned err: %v", err)
	}

	if len(gotServices) != len(retryServices) {
		t.Errorf("retry services = %d, want %d", len(gotServices), len(retryServices))
	}

	gotSteps, err := db.ListStepsForBuild(context.Background(), retryResult.Build, nil, 1, 100)
	if err != nil {
		t.Fatalf("ListStepsForBuild returned err: %v", err)
	}

	if len(gotSteps) != len(retrySteps) {
		t.Errorf("retry steps = %d, want %d", len(gotSteps), len(retrySteps))
	}

	gotLogs, err := db.ListLogsForBuild(context.Background(), retryResult.Build, 1, 100)
	if err != nil {
		t.Fatalf("ListLogsForBuild returned err: %v", err)
	}

	if len(gotLogs) != len(retryServices)+len(retrySteps) {
		t.Errorf("retry logs = %d, want %d", len(gotLogs), len(retryServices)+len(retrySteps))
	}

	_, err = db.PopBuildExecutable(context.Background(), retryResult.Build.GetID())
	if err != nil {
		t.Fatalf("retry PopBuildExecutable returned err: %v", err)
	}
}

func TestEngine_PlanBuild_InvalidChildRollsBack(t *testing.T) {
	db := testPlanEngine(t)
	r := testPlanRepo(t, db)
	b := testPlanBuild(r)
	services := testPlanServices(b)
	steps := testPlanSteps(b)

	// an invalid service (missing image) fails validation inside the transaction
	services[1].SetImage("")

	finalizeCalls := 0

	result, err := db.PlanBuild(context.Background(), b, services, steps, finalizeExecutable([]byte(`{}`), &finalizeCalls))
	if err == nil {
		t.Fatal("PlanBuild should have returned err for an invalid service")
	}

	if result != nil {
		t.Fatal("PlanBuild should not return a result on failure")
	}

	if finalizeCalls != 0 {
		t.Errorf("finalizer should not have been invoked, got %d calls", finalizeCalls)
	}

	count, err := db.CountBuilds(context.Background())
	if err != nil {
		t.Fatalf("CountBuilds returned err: %v", err)
	}

	if count != 0 {
		t.Errorf("CountBuilds after rollback = %d, want 0", count)
	}
}
