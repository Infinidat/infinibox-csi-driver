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

	"github.com/infinidat/infinibox-csi-driver/common"
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
