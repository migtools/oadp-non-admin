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

// Entrypoint of the project
package main

import (
	"reflect"
	"testing"

	_ "github.com/onsi/ginkgo/v2" // To fix: flag provided but not defined: -ginkgo.vv
	"github.com/sirupsen/logrus"
)

func Test_translateLogrusToZapLevel(t *testing.T) {
	for _, level := range logrus.AllLevels {
		t.Run(level.String(), func(t *testing.T) {
			// all levels from logrus are expected to be valid
			zapLevel, invalid := translateLogrusToZapLevel(level)
			if invalid {
				t.Error(level.String() + " from logrus did not convert")
			}
			if zapLevel.String() == "" {
				t.Error("empty zap level unexpected")
			}
		})
	}
}

func TestEncoderForFormat(t *testing.T) {
	tests := []struct {
		name        string
		inputFormat string
		wantTypeStr string
	}{
		{
			name:        "json format",
			inputFormat: "json",
			wantTypeStr: "*zapcore.jsonEncoder",
		},
		{
			name:        "text format",
			inputFormat: "text",
			wantTypeStr: "zapcore.consoleEncoder",
		},
		{
			name:        "invalid format defaults to text",
			inputFormat: "invalid",
			wantTypeStr: "zapcore.consoleEncoder",
		},
		{
			name:        "empty format defaults to text",
			inputFormat: "",
			wantTypeStr: "zapcore.consoleEncoder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := encoderForFormat(tt.inputFormat)
			actualType := reflect.TypeOf(enc).String()
			t.Logf("Returned type for format %q: %T", tt.inputFormat, enc)

			if actualType != tt.wantTypeStr {
				t.Errorf("encoderForFormat(%q) returned encoder of type %s, want %s",
					tt.inputFormat, actualType, tt.wantTypeStr)
			}
		})
	}
}
