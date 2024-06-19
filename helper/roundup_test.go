//go:build unit

package helper

import (
	"testing"
)

// TestRoundUp tests that RoundUp() will handle various sizes in bytes
func TestRoundUp(t *testing.T) {
	tests := []struct {
		byteCount     int64
		wantByteCount int64
	}{
		{0, Bytes1G},
		{140, Bytes1G},
		{Bytes1G - 100, Bytes1G},
		{Bytes1G, Bytes1G},
		{Bytes1Gi - 100, Bytes1Gi},
		{Bytes1Gi, Bytes1Gi},
		{1400405001, 2 * Bytes1Gi},
		{500405001, Bytes1G},
		{980405001, Bytes1G},
		{Bytes1G * 2, Bytes1G * 2},
		{Bytes1Gi * 2, Bytes1Gi * 2},
		{Bytes1Gi * 2.5, Bytes1Gi * 3},
	}

	for _, test := range tests {
		roundValue := RoundUp(test.byteCount)
		if roundValue != test.wantByteCount {
			t.Errorf(`RoundUp("%d") has err not equal: %d, got %d`, test.byteCount, test.wantByteCount, roundValue)
		}
	}
}
