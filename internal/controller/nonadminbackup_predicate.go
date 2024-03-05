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
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

type NonAdminBackupPredicate struct {
	Logger logr.Logger
}

func getNonAdminBackupPredicateLogger(ctx context.Context, name, namespace string) logr.Logger {
	return log.FromContext(ctx).WithValues("NonAdminBackupPredicate", types.NamespacedName{Name: name, Namespace: namespace})
}

func (NonAdminBackupPredicate) Create(ctx context.Context, evt event.CreateEvent) bool {
	if _, ok := evt.Object.(*nacv1alpha1.NonAdminBackup); ok {
		nameSpace := evt.Object.GetNamespace()
		name := evt.Object.GetName()
		logger := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
		logger.V(1).Info("Received Create NonAdminBackupPredicate")
		return true
	}

	return false
}

func (NonAdminBackupPredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	if _, ok := evt.ObjectNew.(*nacv1alpha1.NonAdminBackup); ok {
		nameSpace := evt.ObjectNew.GetNamespace()
		name := evt.ObjectNew.GetName()
		logger := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
		logger.V(1).Info("Received Update NonAdminBackupPredicate")
		return true
	}
	return false
}

func (NonAdminBackupPredicate) Delete(ctx context.Context, evt event.DeleteEvent) bool {
	if _, ok := evt.Object.(*nacv1alpha1.NonAdminBackup); ok {
		nameSpace := evt.Object.GetNamespace()
		name := evt.Object.GetName()
		logger := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
		logger.V(1).Info("Received Delete NonAdminBackupPredicate")
		return true
	}
	return false
}

func (NonAdminBackupPredicate) Generic(ctx context.Context, evt event.GenericEvent) bool {
	if _, ok := evt.Object.(*nacv1alpha1.NonAdminBackup); ok {
		nameSpace := evt.Object.GetNamespace()
		name := evt.Object.GetName()
		logger := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
		logger.V(1).Info("Received Generic NonAdminBackupPredicate")
		return true
	}
	return false
}
