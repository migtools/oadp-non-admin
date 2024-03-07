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

// Package constant contains all common constants used in the project
package constant

// Common labels for objects manipulated by the Non Admin Controller
// Labels should be used to identify the NAC backup
// Annotations on the other hand should be used to define ownership
// of the specific Object, such as Backup.
const (
	OadpLabel                    = "openshift.io/oadp" // TODO import?
	ManagedByLabel               = "app.kubernetes.io/managed-by"
	ManagedByLabelValue          = "oadp-nac-controller" // TODO why not use same project name as in PROJECT file?
	NabOriginNameAnnotation      = "openshift.io/oadp-nab-origin-name"
	NabOriginNamespaceAnnotation = "openshift.io/oadp-nab-origin-namespace"
	NabOriginUUIDAnnotation      = "openshift.io/oadp-nab-origin-uuid"
)
