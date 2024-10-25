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

import "k8s.io/apimachinery/pkg/util/validation"

// Common labels for objects manipulated by the Non Admin Controller
// Labels should be used to identify the NAC object
// Annotations on the other hand should be used to define ownership
// of the specific Object, such as Backup/Restore.
const (
	OadpLabel                    = "openshift.io/oadp" // TODO import?
	OadpLabelValue               = TrueString
	ManagedByLabel               = "app.kubernetes.io/managed-by"
	ManagedByLabelValue          = "oadp-nac-controller" // TODO why not use same project name as in PROJECT file?
	NabOriginNameAnnotation      = "openshift.io/oadp-nab-origin-name"
	NabOriginNamespaceAnnotation = "openshift.io/oadp-nab-origin-namespace"
	NabOriginNameUUIDLabel       = "openshift.io/oadp-nab-origin-nameuuid"
	NarOriginNameUUIDLabel       = "openshift.io/oadp-nar-origin-nameuuid"
	NabFinalizerName             = "nonadminbackup.oadp.openshift.io/finalizer"
)

// Common environment variables for the Non Admin Controller
const (
	NamespaceEnvVar = "WATCH_NAMESPACE"
)

// EmptyString defines a constant for the empty string
const EmptyString = ""

// NameDelimiter defines character that is used to separate name parts
const NameDelimiter = "-"

// TrueString defines a constant for the True string
const TrueString = "True"

// MaximumNacObjectNameLength represents Generated Non Admin Object Name and
// must be below 63 characters, because it's used within object Label Value
const MaximumNacObjectNameLength = validation.DNS1123LabelMaxLength
