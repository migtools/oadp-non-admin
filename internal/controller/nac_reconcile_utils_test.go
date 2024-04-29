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

import (
	"errors"
	"testing"
)

func sampleReconcileFunc1(_ ...any) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	return false, false, nil
}

func sampleReconcileFunc2(_ ...any) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	return true, false, nil
}

func sampleReconcileFunc3(_ ...any) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	return false, true, nil
}

func sampleReconcileFuncWithError(_ ...any) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	return false, false, errors.New("sample error")
}

func sampleConflictingReconcileFunc(_ ...any) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	return true, true, nil
}

func TestReconcileBatch(t *testing.T) {
	tests := []struct {
		expectedError   error
		name            string
		reconcileFuncs  []ReconcileFunc
		expectedExit    bool
		expectedRequeue bool
	}{
		{
			name:            "No functions",
			reconcileFuncs:  nil,
			expectedExit:    false,
			expectedRequeue: false,
			expectedError:   nil,
		},
		{
			name:            "All functions return false",
			reconcileFuncs:  []ReconcileFunc{sampleReconcileFunc1, sampleReconcileFunc1},
			expectedExit:    false,
			expectedRequeue: false,
			expectedError:   nil,
		},
		{
			name:            "First function returns true",
			reconcileFuncs:  []ReconcileFunc{sampleReconcileFunc2, sampleReconcileFunc1},
			expectedExit:    true,
			expectedRequeue: false,
			expectedError:   nil,
		},
		{
			name:            "Second function returns requeue",
			reconcileFuncs:  []ReconcileFunc{sampleReconcileFunc1, sampleReconcileFunc3},
			expectedExit:    false,
			expectedRequeue: true,
			expectedError:   nil,
		},
		{
			name:            "Function returns error",
			reconcileFuncs:  []ReconcileFunc{sampleReconcileFunc1, sampleReconcileFuncWithError},
			expectedExit:    false,
			expectedRequeue: false,
			expectedError:   errors.New("sample error"),
		},
		{
			name:            "Function signals conflicting exit and requeue",
			reconcileFuncs:  []ReconcileFunc{sampleConflictingReconcileFunc},
			expectedExit:    true,
			expectedRequeue: false,
			expectedError:   errors.New("conflicting exit and requeue signals - can not be both true"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exit, requeue, err := ReconcileBatch(test.reconcileFuncs...)
			if exit != test.expectedExit {
				t.Errorf("Expected exit: %v, but got: %v", test.expectedExit, exit)
			}
			if requeue != test.expectedRequeue {
				t.Errorf("Expected requeue: %v, but got: %v", test.expectedRequeue, requeue)
			}
			if (err == nil && test.expectedError != nil) || (err != nil && test.expectedError == nil) || (err != nil && test.expectedError != nil && err.Error() != test.expectedError.Error()) {
				t.Errorf("Expected error: %v, but got: %v", test.expectedError, err)
			}
		})
	}
}
