package controller

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeUniqueKeyTOfTMaps(t *testing.T) {
	type args struct {
		userLabels []map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "append unique labels together",
			args: args{
				userLabels: []map[string]string{
					{"a": "a"},
					{"b": "b"},
				},
			},
			want: map[string]string{
				"a": "a",
				"b": "b",
			},
		},
		{
			name: "append unique labels together, with valid duplicates",
			args: args{
				userLabels: []map[string]string{
					{"a": "a"},
					{"b": "b"},
					{"b": "b"},
				},
			},
			want: map[string]string{
				"a": "a",
				"b": "b",
			},
		},
		{
			name: "append unique labels together - nil sandwich",
			args: args{
				userLabels: []map[string]string{
					{"a": "a"},
					nil,
					{"b": "b"},
				},
			},
			want: map[string]string{
				"a": "a",
				"b": "b",
			},
		},
		{
			name: "should error when append duplicate label keys with different value together",
			args: args{
				userLabels: []map[string]string{
					{"a": "a"},
					{"a": "b"},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeUniqueKeyTOfTMaps(tt.args.userLabels...)
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
	ownerUuid := "f2c4d2c3-58d3-46ec-bf03-5940f567f7f8"

	expectedAnnotations := map[string]string{
		NabOriginNamespaceAnnotation: ownerNamespace,
		NabOriginNameAnnotation:      ownerName,
		NabOriginUuidAnnotation:      ownerUuid,
		"existingKey1":               "existingValue1",
		"existingKey2":               "existingValue2",
	}

	mergedAnnotations := CreateAnnotationsForNac(ownerNamespace, ownerName, ownerUuid, existingAnnotations)
	assert.Equal(t, expectedAnnotations, mergedAnnotations, "Merged annotations should match expected annotations")

	// Merging annotations with conflicts
	existingAnnotationsWithConflict := map[string]string{
		NabOriginNameAnnotation: "conflictingValue",
	}

	expectedAnnotationsWithConflict := map[string]string{
		NabOriginNameAnnotation:      ownerName,
		NabOriginNamespaceAnnotation: ownerNamespace,
		NabOriginUuidAnnotation:      ownerUuid,
	}

	mergedAnnotationsWithConflict := CreateAnnotationsForNac(ownerNamespace, ownerName, ownerUuid, existingAnnotationsWithConflict)
	assert.Equal(t, expectedAnnotationsWithConflict, mergedAnnotationsWithConflict, "Merged annotations should match expected annotations with conflict")
}
