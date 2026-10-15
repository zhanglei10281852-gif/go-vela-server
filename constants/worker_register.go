// SPDX-License-Identifier: Apache-2.0

package constants

// Worker register token statuses.
//
// A register token authorizes exactly one worker registration. The
// registration establishes a single worker lifecycle generation: the
// register token's generation identifier is recorded on the worker row
// and is embedded in every auth token minted for that generation.
const (
	// WorkerRegisterTokenStatusActive defines the status for a register
	// token that has been issued but not yet consumed by a registration.
	WorkerRegisterTokenStatusActive = "active"

	// WorkerRegisterTokenStatusUsed defines the status for a register
	// token that has already established a worker generation. It can
	// never establish another generation.
	WorkerRegisterTokenStatusUsed = "used"
)
