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
	"fmt"
)

// Common labels for objects manipulated by the Non Admin Controller
// Labels should be used to identify the NAC backup
// Annotations on the other hand should be used to define ownership
// of the specific Object, such as Backup.
const (
	OadpLabel                    = "openshift.io/oadp"
	ManagedByLabel               = "app.kubernetes.io/managed-by"
	ManagedByLabelValue          = "oadp-nac-controller"
	NabOriginNameAnnotation      = "openshift.io/oadp-nab-origin-name"
	NabOriginNamespaceAnnotation = "openshift.io/oadp-nab-origin-namespace"
	NabOriginUUIDAnnotation      = "openshift.io/oadp-nab-origin-uuid"
)

const (
	OadpNamespace = "openshift-adp"
)

func CreateLabelsForNac(labels map[string]string) map[string]string {
	defaultLabels := map[string]string{
		OadpLabel:      "True",
		ManagedByLabel: ManagedByLabelValue,
	}

	mergedLabels, err := mergeUniqueKeyTOfTMaps(defaultLabels, labels)
	if err != nil {
		// TODO logger
		_, _ = fmt.Println("Error merging labels:", err)
		return defaultLabels
	}
	return mergedLabels
}

func CreateAnnotationsForNac(ownerNamespace string, ownerName string, ownerUUID string, existingAnnotations map[string]string) map[string]string {
	defaultAnnotations := map[string]string{
		NabOriginNamespaceAnnotation: ownerNamespace,
		NabOriginNameAnnotation:      ownerName,
		NabOriginUUIDAnnotation:      ownerUUID,
	}

	mergedAnnotations, err := mergeUniqueKeyTOfTMaps(defaultAnnotations, existingAnnotations)
	if err != nil {
		// TODO logger
		_, _ = fmt.Println("Error merging annotations:", err)
		return defaultAnnotations
	}
	return mergedAnnotations
}

// Similar to as pkg/common/common.go:AppendUniqueKeyTOfTMaps from github.com/openshift/oadp-operator
func mergeUniqueKeyTOfTMaps[T comparable](userMap ...map[T]T) (map[T]T, error) {
	var base map[T]T
	for i, mapElements := range userMap {
		if mapElements == nil {
			continue
		}
		if base == nil {
			base = make(map[T]T)
		}
		for k, v := range mapElements {
			existingValue, found := base[k]
			if found {
				if existingValue != v {
					return nil, fmt.Errorf("conflicting key %v with value %v in map %d may not override %v", k, v, i, existingValue)
				}
			} else {
				base[k] = v
			}
		}
	}
	return base, nil
}
