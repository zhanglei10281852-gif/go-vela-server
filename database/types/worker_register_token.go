// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql"
	"errors"

	api "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/util"
)

var (
	// ErrEmptyWorkerRegisterTokenID defines the error type when a
	// WorkerRegisterToken type has an empty TokenID field provided.
	ErrEmptyWorkerRegisterTokenID = errors.New("empty worker register token id provided")

	// ErrEmptyWorkerRegisterTokenHost defines the error type when a
	// WorkerRegisterToken type has an empty Hostname field provided.
	ErrEmptyWorkerRegisterTokenHost = errors.New("empty worker register token hostname provided")

	// ErrEmptyWorkerRegisterTokenStatus defines the error type when a
	// WorkerRegisterToken type has an empty Status field provided.
	ErrEmptyWorkerRegisterTokenStatus = errors.New("empty worker register token status provided")
)

// WorkerRegisterToken is the database representation of a single-use
// worker registration token. Each row authorizes one registration of
// one worker lifecycle generation for a hostname.
type WorkerRegisterToken struct {
	ID        sql.NullInt64  `sql:"id"`
	TokenID   sql.NullString `sql:"token_id"`
	Hostname  sql.NullString `sql:"hostname"`
	Status    sql.NullString `sql:"status"`
	WorkerID  sql.NullInt64  `sql:"worker_id"`
	ExpiresAt sql.NullInt64  `sql:"expires_at"`
	CreatedAt sql.NullInt64  `sql:"created_at"`
}

// Nullify ensures the valid flag for
// the sql.Null types are properly set.
//
// When a field within the WorkerRegisterToken type is the zero
// value for the field, the valid flag is set to false causing it
// to be NULL in the database.
func (r *WorkerRegisterToken) Nullify() *WorkerRegisterToken {
	if r == nil {
		return nil
	}

	// check if the ID field should be false
	if r.ID.Int64 == 0 {
		r.ID.Valid = false
	}

	// check if the TokenID field should be false
	if len(r.TokenID.String) == 0 {
		r.TokenID.Valid = false
	}

	// check if the Hostname field should be false
	if len(r.Hostname.String) == 0 {
		r.Hostname.Valid = false
	}

	// check if the Status field should be false
	if len(r.Status.String) == 0 {
		r.Status.Valid = false
	}

	// check if the WorkerID field should be false
	if r.WorkerID.Int64 == 0 {
		r.WorkerID.Valid = false
	}

	// check if the ExpiresAt field should be false
	if r.ExpiresAt.Int64 == 0 {
		r.ExpiresAt.Valid = false
	}

	// check if the CreatedAt field should be false
	if r.CreatedAt.Int64 == 0 {
		r.CreatedAt.Valid = false
	}

	return r
}

// ToAPI converts the WorkerRegisterToken type
// to an API WorkerRegisterToken type.
func (r *WorkerRegisterToken) ToAPI() *api.WorkerRegisterToken {
	token := new(api.WorkerRegisterToken)

	token.SetID(r.ID.Int64)
	token.SetTokenID(r.TokenID.String)
	token.SetHostname(r.Hostname.String)
	token.SetStatus(r.Status.String)
	token.SetWorkerID(r.WorkerID.Int64)
	token.SetExpiresAt(r.ExpiresAt.Int64)
	token.SetCreatedAt(r.CreatedAt.Int64)

	return token
}

// Validate verifies the necessary fields for the
// WorkerRegisterToken type are populated correctly.
func (r *WorkerRegisterToken) Validate() error {
	// verify the TokenID field is populated
	if len(r.TokenID.String) == 0 {
		return ErrEmptyWorkerRegisterTokenID
	}

	// verify the Hostname field is populated
	if len(r.Hostname.String) == 0 {
		return ErrEmptyWorkerRegisterTokenHost
	}

	// verify the Status field is populated
	if len(r.Status.String) == 0 {
		return ErrEmptyWorkerRegisterTokenStatus
	}

	// ensure that all string fields that can be returned as JSON
	// are sanitized to avoid unsafe HTML content
	r.TokenID = sql.NullString{String: util.Sanitize(r.TokenID.String), Valid: r.TokenID.Valid}
	r.Hostname = sql.NullString{String: util.Sanitize(r.Hostname.String), Valid: r.Hostname.Valid}
	r.Status = sql.NullString{String: util.Sanitize(r.Status.String), Valid: r.Status.Valid}

	return nil
}

// WorkerRegisterTokenFromAPI converts the API worker register token
// type to a database worker register token type.
func WorkerRegisterTokenFromAPI(r *api.WorkerRegisterToken) *WorkerRegisterToken {
	token := &WorkerRegisterToken{
		ID:        sql.NullInt64{Int64: r.GetID(), Valid: true},
		TokenID:   sql.NullString{String: r.GetTokenID(), Valid: true},
		Hostname:  sql.NullString{String: r.GetHostname(), Valid: true},
		Status:    sql.NullString{String: r.GetStatus(), Valid: true},
		WorkerID:  sql.NullInt64{Int64: r.GetWorkerID(), Valid: true},
		ExpiresAt: sql.NullInt64{Int64: r.GetExpiresAt(), Valid: true},
		CreatedAt: sql.NullInt64{Int64: r.GetCreatedAt(), Valid: true},
	}

	return token.Nullify()
}
