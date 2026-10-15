// SPDX-License-Identifier: Apache-2.0

package types

// WorkerRegisterToken is the internal representation of a single-use
// worker registration token.
//
// Each register token authorizes exactly one registration for a single
// worker hostname and establishes exactly one worker lifecycle
// generation. After the generation has been established the token is
// marked as used and cannot establish any other generation; deleting
// the worker revokes every register token issued for that hostname.
//
// This type is never exposed through the API.
type WorkerRegisterToken struct {
	ID        int64  `json:"-"`
	TokenID   string `json:"-"`
	Hostname  string `json:"-"`
	Status    string `json:"-"`
	WorkerID  int64  `json:"-"`
	ExpiresAt int64  `json:"-"`
	CreatedAt int64  `json:"-"`
}

// GetID returns the ID field.
func (r *WorkerRegisterToken) GetID() int64 {
	if r == nil {
		return 0
	}

	return r.ID
}

// GetTokenID returns the TokenID field. This is the opaque generation
// identifier embedded in the signed register token.
func (r *WorkerRegisterToken) GetTokenID() string {
	if r == nil {
		return ""
	}

	return r.TokenID
}

// GetHostname returns the Hostname field.
func (r *WorkerRegisterToken) GetHostname() string {
	if r == nil {
		return ""
	}

	return r.Hostname
}

// GetStatus returns the Status field.
func (r *WorkerRegisterToken) GetStatus() string {
	if r == nil {
		return ""
	}

	return r.Status
}

// GetWorkerID returns the ID of the worker row that consumed the token.
func (r *WorkerRegisterToken) GetWorkerID() int64 {
	if r == nil {
		return 0
	}

	return r.WorkerID
}

// GetExpiresAt returns the ExpiresAt field.
func (r *WorkerRegisterToken) GetExpiresAt() int64 {
	if r == nil {
		return 0
	}

	return r.ExpiresAt
}

// GetCreatedAt returns the CreatedAt field.
func (r *WorkerRegisterToken) GetCreatedAt() int64 {
	if r == nil {
		return 0
	}

	return r.CreatedAt
}

// SetID sets the ID field.
func (r *WorkerRegisterToken) SetID(v int64) {
	if r == nil {
		return
	}

	r.ID = v
}

// SetTokenID sets the TokenID field.
func (r *WorkerRegisterToken) SetTokenID(v string) {
	if r == nil {
		return
	}

	r.TokenID = v
}

// SetHostname sets the Hostname field.
func (r *WorkerRegisterToken) SetHostname(v string) {
	if r == nil {
		return
	}

	r.Hostname = v
}

// SetStatus sets the Status field.
func (r *WorkerRegisterToken) SetStatus(v string) {
	if r == nil {
		return
	}

	r.Status = v
}

// SetWorkerID sets the WorkerID field.
func (r *WorkerRegisterToken) SetWorkerID(v int64) {
	if r == nil {
		return
	}

	r.WorkerID = v
}

// SetExpiresAt sets the ExpiresAt field.
func (r *WorkerRegisterToken) SetExpiresAt(v int64) {
	if r == nil {
		return
	}

	r.ExpiresAt = v
}

// SetCreatedAt sets the CreatedAt field.
func (r *WorkerRegisterToken) SetCreatedAt(v int64) {
	if r == nil {
		return
	}

	r.CreatedAt = v
}
