// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"reflect"
	"testing"

	api "github.com/go-vela/server/api/types"
)

func TestWorker_ActiveWorkerResp(t *testing.T) {
	testWorker := api.Worker{}

	err := json.Unmarshal([]byte(WorkerResp), &testWorker)
	if err != nil {
		t.Errorf("error unmarshaling worker: %v", err)
	}

	tWorker := reflect.TypeFor[api.Worker]()

	for i := 0; i < tWorker.NumField(); i++ {
		// fields excluded from JSON (json:"-") cannot be populated by
		// unmarshaling and are not part of the response contract
		if name, ok := tWorker.Field(i).Tag.Lookup("json"); ok && name == "-" {
			continue
		}

		if reflect.ValueOf(testWorker).Field(i).IsNil() {
			t.Errorf("WorkerResp missing field %s", tWorker.Field(i).Name)
		}
	}
}

func TestWorker_ListActiveWorkerResp(t *testing.T) {
	testWorkers := []api.Worker{}

	err := json.Unmarshal([]byte(WorkersResp), &testWorkers)
	if err != nil {
		t.Errorf("error unmarshaling worker: %v", err)
	}

	for index, worker := range testWorkers {
		tWorker := reflect.TypeFor[api.Worker]()

		for i := 0; i < tWorker.NumField(); i++ {
			// fields excluded from JSON (json:"-") cannot be populated by
			// unmarshaling and are not part of the response contract
			if name, ok := tWorker.Field(i).Tag.Lookup("json"); ok && name == "-" {
				continue
			}

			if reflect.ValueOf(worker).Field(i).IsNil() {
				t.Errorf("WorkersResp index %d missing field %s", index, tWorker.Field(i).Name)
			}
		}
	}
}
