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

// Package source contains all event sources of the project
package source

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
)

// PeriodicalSource will periodically add an empty object to controller queue
type PeriodicalSource struct {
	Frequency time.Duration
}

// Start periodically adds an empty object to queue
func (p PeriodicalSource) Start(ctx context.Context, q workqueue.RateLimitingInterface) error { //nolint:unparam // object must implement function with this signature
	go wait.Until(func() {
		q.Add(ctrl.Request{})
	}, p.Frequency, ctx.Done())

	return nil
}
