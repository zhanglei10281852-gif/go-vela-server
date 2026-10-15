// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"

	"github.com/sirupsen/logrus"

	api "github.com/go-vela/server/api/types"
	"github.com/go-vela/server/constants"
	"github.com/go-vela/server/database/types"
)

// CreateWorkerRegisterToken stores a newly issued, single-use worker
// registration token. The token's ID is the generation identifier the
// signed register token carries; it stays "active" until exactly one
// registration consumes it.
func (e *Engine) CreateWorkerRegisterToken(ctx context.Context, r *api.WorkerRegisterToken) (*api.WorkerRegisterToken, error) {
	e.logger.WithFields(logrus.Fields{
		"worker": r.GetHostname(),
	}).Tracef("creating worker register token for worker %s", r.GetHostname())

	// cast the API type to database type
	token := types.WorkerRegisterTokenFromAPI(r)

	// validate the necessary fields are populated
	err := token.Validate()
	if err != nil {
		return nil, err
	}

	// send query to the database
	result := e.client.
		WithContext(ctx).
		Table(constants.TableWorkerRegisterToken).
		Create(token)

	return token.ToAPI(), result.Error
}

// GetWorkerRegisterToken gets a worker register token by its token ID
// (the generation identifier embedded in the signed register token).
func (e *Engine) GetWorkerRegisterToken(ctx context.Context, tokenID string) (*api.WorkerRegisterToken, error) {
	e.logger.WithFields(logrus.Fields{
		"token_id": tokenID,
	}).Tracef("getting worker register token %s", tokenID)

	// variable to store query results
	r := new(types.WorkerRegisterToken)

	// send query to the database and store result in variable
	err := e.client.
		WithContext(ctx).
		Table(constants.TableWorkerRegisterToken).
		Where("token_id = ?", tokenID).
		Take(r).
		Error
	if err != nil {
		return nil, err
	}

	// return the register token
	return r.ToAPI(), nil
}

// DeleteWorkerRegisterToken deletes a worker register token by its
// token ID. It is used to roll back a registration that could not be
// completed (for example, when the signed token could not be minted).
func (e *Engine) DeleteWorkerRegisterToken(ctx context.Context, tokenID string) error {
	e.logger.WithFields(logrus.Fields{
		"token_id": tokenID,
	}).Tracef("deleting worker register token %s", tokenID)

	return e.client.
		WithContext(ctx).
		Table(constants.TableWorkerRegisterToken).
		Where("token_id = ?", tokenID).
		Delete(new(types.WorkerRegisterToken)).
		Error
}
