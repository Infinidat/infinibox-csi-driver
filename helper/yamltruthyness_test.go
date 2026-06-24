//go:build unit

/*
Copyright 2026 Infinidat
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

package helper

import (
	"strings"
	"testing"
)

// ErrorContains checks if the error message in 'err' contains the text in
// want. This is safe when out is nil. Use an empty string for want
// if you want to test that err is nil.
// Ref: https://stackoverflow.com/questions/42035104/how-to-unit-test-go-errors
func ErrorContains(err error, want string) bool {
	if err == nil {
		return want == ""
	}
	if want == "" {
		return false
	}
	return strings.Contains(err.Error(), want)
}

// TestYamlBoolToBool tests that YamlBoolToBool() generates bools.
func TestYamlBoolToBool(t *testing.T) {
	expected_err := "not a valid YAML boolean"
	tests := []struct {
		input   string
		want    bool
		wanterr string
	}{
		{"y", true, ""},
		{"yes", true, ""},
		{"n", false, ""},
		{"OFF", false, ""},
		{"?", false, expected_err},
		{"", false, expected_err},
		{"  ", false, expected_err},
		{"yesno", false, expected_err},
		{"yEs", false, expected_err},
		{"nO", false, expected_err},
		{"0", false, expected_err},
		{"1", false, expected_err},
	}

	for _, test := range tests {
		answer, err := YamlBoolToBool(test.input)
		if !ErrorContains(err, test.wanterr) {
			t.Errorf(`YamlBoolToBool("%s") has err: %s`, test.input, err)
		}
		if answer != test.want {
			t.Errorf(`YamlBoolToBool("%s") != %t`, test.input, test.want)
		}
	}
}
