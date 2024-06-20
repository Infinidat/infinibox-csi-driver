//go:build unit

package helper

import (
	"infinibox-csi-driver/common"
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
		{common.BytesInOneGibibyte - 100, common.BytesInOneGibibyte},
		{common.BytesInOneGibibyte, common.BytesInOneGibibyte},
		{1400405001, 2 * common.BytesInOneGibibyte},
		{500405001, Bytes1G},
		{980405001, Bytes1G},
		{Bytes1G * 2, Bytes1G * 2},
		{common.BytesInOneGibibyte * 2, common.BytesInOneGibibyte * 2},
		{common.BytesInOneGibibyte * 2.5, common.BytesInOneGibibyte * 3},
	}

	for _, test := range tests {
		roundValue := RoundUp(test.byteCount)
		if roundValue != test.wantByteCount {
			t.Errorf(`RoundUp("%d") has err not equal: %d, got %d`, test.byteCount, test.wantByteCount, roundValue)
		}
	}
}
