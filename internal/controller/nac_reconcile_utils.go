/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import "errors"

// ReconcileFunc represents a function that performs a reconciliation operation.
// Parameters:
//   - ...any: A variadic number of arguments,
//     allowing for flexibility in passing parameters to the reconciliation function.
//
// Returns:
//   - bool: Indicates whether the reconciliation process should exit.
//   - bool: Indicates whether the reconciliation process should requeue.
//   - error: An error encountered during reconciliation, if any.
type ReconcileFunc func(...any) (bool, bool, error)

// ReconcileBatch executes a batch of reconcile functions sequentially,
// allowing for complex reconciliation processes in a controlled manner.
// It iterates through the provided reconcile functions until one of the
// following conditions is met:
//   - A function signals the need to exit the reconciliation process.
//   - A function signals the need to exit and requeue the reconciliation process.
//   - An error occurs during reconciliation.
//
// If any reconcile function indicates a need to exit or requeue,
// the function immediately returns with the respective exit and requeue
// flags set. If an error occurs during any reconciliation function call,
// it is propagated up and returned. If none of the reconcile functions
// indicate a need to exit, requeue, or result in an error, the function
// returns false for both exit and requeue flags, indicating successful reconciliation.
//
// If a reconcile function signals both the need to exit and requeue, indicating
// conflicting signals, the function returns an error with the exit flag set to true
// and the requeue flag set to false, so no .
//
// Parameters:
//   - reconcileFuncs: A list of ReconcileFunc functions representing
//     the reconciliation steps to be executed sequentially.
//
// Returns:
//   - bool: Indicates whether the reconciliation process should exit.
//   - bool: Indicates whether the reconciliation process should requeue.
//   - error: An error encountered during reconciliation, if any.
func ReconcileBatch(reconcileFuncs ...ReconcileFunc) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	var exit, requeue bool
	var err error
	var conflictError = errors.New("conflicting exit and requeue signals - can not be both true")

	for _, f := range reconcileFuncs {
		exit, requeue, err = f()

		// Check if both exit and requeue are true
		// If this happens do not requeue, but exit with error
		if exit && requeue {
			return true, false, conflictError
		}

		// Return if there is a need to exit, requeue, or an error occurred
		if exit || requeue || err != nil {
			return exit, requeue, err
		}
	}

	// Do not requeue or exit reconcile. Also no error occurred.
	return false, false, nil
}
