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

import (
	"github.com/openshift/oadp-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Common labels for objects manipulated by the Non Admin Controller
// Labels should be used to identify the NAC object
// Annotations on the other hand should be used to define ownership
// of the specific Object, such as Backup/Restore.
const (
	OadpLabel               = v1alpha1.OadpOperatorLabel
	OadpLabelValue          = TrueString
	ManagedByLabel          = "app.kubernetes.io/managed-by"
	ManagedByLabelValue     = "oadp-nac-controller" // TODO why not use same project name as in PROJECT file?
	NabOriginNACUUIDLabel   = v1alpha1.OadpOperatorLabel + "-nab-origin-nacuuid"
	NarOriginNACUUIDLabel   = v1alpha1.OadpOperatorLabel + "-nar-origin-nacuuid"
	NabslOriginNACUUIDLabel = v1alpha1.OadpOperatorLabel + "-nabsl-origin-nacuuid"
	NabSyncLabel            = v1alpha1.OadpOperatorLabel + "-nab-synced-from-nacuuid"

	NabOriginNameAnnotation        = v1alpha1.OadpOperatorLabel + "-nab-origin-name"
	NabOriginNamespaceAnnotation   = v1alpha1.OadpOperatorLabel + "-nab-origin-namespace"
	NarOriginNameAnnotation        = v1alpha1.OadpOperatorLabel + "-nar-origin-name"
	NarOriginNamespaceAnnotation   = v1alpha1.OadpOperatorLabel + "-nar-origin-namespace"
	NabslOriginNameAnnotation      = v1alpha1.OadpOperatorLabel + "-nabsl-origin-name"
	NabslOriginNamespaceAnnotation = v1alpha1.OadpOperatorLabel + "-nabsl-origin-namespace"
	NadrOriginNameAnnotation       = v1alpha1.OadpOperatorLabel + "-nadr-origin-name"
	NadrOriginNamespaceAnnotation  = v1alpha1.OadpOperatorLabel + "-nadr-origin-namespace"

	NabFinalizerName   = "nonadminbackup.oadp.openshift.io/finalizer"
	NarFinalizerName   = "nonadminrestore.oadp.openshift.io/finalizer"
	NabslFinalizerName = "nonadminbackupstoragelocation.oadp.openshift.io/finalizer"
)

// Common environment variables for the Non Admin Controller
const (
	NamespaceEnvVar = "WATCH_NAMESPACE"
	// Numeric Log Level corresponding to logrus levels (matching velero).
	// 0 = panic
	// 1 = Fatal
	// 2 = Error
	// 3 = Warn
	// 4 = Info
	// 5 = Debug
	// 6 = Trace
	LogLevelEnvVar = "LOG_LEVEL"
)

// EmptyString defines a constant for the empty string
const EmptyString = ""

// NameDelimiter defines character that is used to separate name parts
const NameDelimiter = "-"

// TrueString defines a constant for the True string
const TrueString = "True"

// NamespaceString defines a constant for the Namespace string
const NamespaceString = "Namespace"

// NameString defines a constant for the Name string
const NameString = "name"

// CurrentPhaseString defines a constant for the Current Phase string
const CurrentPhaseString = "currentPhase"

// UUIDString defines a constant for the UUID string
const UUIDString = "UUID"

// JSONTagString defines a constant for the JSON tag string
const JSONTagString = "json"

// CommaString defines a constant for the comma string
const CommaString = ","

// MaximumNacObjectNameLength represents Generated Non Admin Object Name and
// must be below 63 characters, because it's used within object Label Value
const MaximumNacObjectNameLength = validation.DNS1123LabelMaxLength

// NABRestrictedErr holds an error message template for a non-admin backup operation that is restricted.
const NABRestrictedErr = "NonAdminBackup %s is restricted"

// NARRestrictedErr holds an error message template for a non-admin restore operation that is restricted.
const NARRestrictedErr = "NonAdminRestore %s is restricted"

// Magic numbers
const (
	Base10 = 10
	Bits32 = 32
)
