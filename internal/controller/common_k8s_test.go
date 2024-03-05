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
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeUniqueKeyTOfTMaps(t *testing.T) {
	const (
		d     = "d"
		delta = "delta"
	)
	tests := []struct {
		name    string
		want    map[string]string
		args    []map[string]string
		wantErr bool
	}{
		{
			name: "append unique labels together",
			args: []map[string]string{
				{"a": "alpha"},
				{"b": "beta"},
			},
			want: map[string]string{
				"a": "alpha",
				"b": "beta",
			},
		},
		{
			name: "append unique labels together, with valid duplicates",
			args: []map[string]string{
				{"c": "gamma"},
				{d: delta},
				{d: delta},
			},
			want: map[string]string{
				"c": "gamma",
				d:   delta,
			},
		},
		{
			name: "append unique labels together - nil sandwich",
			args: []map[string]string{
				{"x": "chi"},
				nil,
				{"y": "psi"},
			},
			want: map[string]string{
				"x": "chi",
				"y": "psi",
			},
		},
		{
			name: "should error when append duplicate label keys with different value together",
			args: []map[string]string{
				{"key": "value-1"},
				{"key": "value-2"},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeUniqueKeyTOfTMaps(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("mergeUniqueKeyTOfTMaps() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeUniqueKeyTOfTMaps() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateLabelsForNac(t *testing.T) {
	// Additional labels provided
	additionalLabels := map[string]string{
		"additionalLabel1": "value1",
		"additionalLabel2": "value2",
	}

	expectedLabels := map[string]string{
		OadpLabel:          "True",
		ManagedByLabel:     "oadp-nac-controller",
		"additionalLabel1": "value1",
		"additionalLabel2": "value2",
	}

	mergedLabels := CreateLabelsForNac(additionalLabels)
	assert.Equal(t, expectedLabels, mergedLabels, "Merged labels should match expected labels")
}

func TestCreateAnnotationsForNac(t *testing.T) {
	// Merging annotations without conflicts
	existingAnnotations := map[string]string{
		"existingKey1": "existingValue1",
		"existingKey2": "existingValue2",
	}

	ownerName := "testOwner"
	ownerNamespace := "testNamespace"
	ownerUUID := "f2c4d2c3-58d3-46ec-bf03-5940f567f7f8"

	expectedAnnotations := map[string]string{
		NabOriginNamespaceAnnotation: ownerNamespace,
		NabOriginNameAnnotation:      ownerName,
		NabOriginUUIDAnnotation:      ownerUUID,
		"existingKey1":               "existingValue1",
		"existingKey2":               "existingValue2",
	}

	mergedAnnotations := CreateAnnotationsForNac(ownerNamespace, ownerName, ownerUUID, existingAnnotations)
	assert.Equal(t, expectedAnnotations, mergedAnnotations, "Merged annotations should match expected annotations")

	// Merging annotations with conflicts
	existingAnnotationsWithConflict := map[string]string{
		NabOriginNameAnnotation: "conflictingValue",
	}

	expectedAnnotationsWithConflict := map[string]string{
		NabOriginNameAnnotation:      ownerName,
		NabOriginNamespaceAnnotation: ownerNamespace,
		NabOriginUUIDAnnotation:      ownerUUID,
	}

	mergedAnnotationsWithConflict := CreateAnnotationsForNac(ownerNamespace, ownerName, ownerUUID, existingAnnotationsWithConflict)
	assert.Equal(t, expectedAnnotationsWithConflict, mergedAnnotationsWithConflict, "Merged annotations should match expected annotations with conflict")
}
