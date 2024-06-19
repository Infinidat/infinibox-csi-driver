/*
Copyright 2022 Infinidat
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

// 1Gi is the default minimum for any input value
const Bytes1Gi = 1073741824

const Bytes1G = 1000000000

func RoundUp(input int64) (output int64) {

	//return the minimum (1G) if input value is less than the min
	if input < Bytes1G {
		zlog.Debug().Msgf("number %d less than minimum %d\n", input, Bytes1G)
		return Bytes1G
	}

	//return the minimum (1Gi) if input value is less than the min
	if input > Bytes1G && input < Bytes1Gi {
		zlog.Debug().Msgf("number %d less than minimum %d\n", input, Bytes1Gi)
		return Bytes1Gi
	}

	// test for valid increments of G
	incrementsOf1G := input % Bytes1G
	zlog.Debug().Msgf("bytes 1G increments %d\n", incrementsOf1G)
	if incrementsOf1G == 0 {
		zlog.Debug().Msgf("valid increment of %d\n", Bytes1G)
		return input
	}

	// test for valid increments of Gi
	incrementsOf1Gi := input % Bytes1Gi
	zlog.Debug().Msgf("bytes 1Gi increments %d\n", incrementsOf1Gi)
	if incrementsOf1Gi == 0 {
		zlog.Debug().Msgf("valid increment of %d\n", Bytes1Gi)
		return input
	}

	// some fractional amount was specified, we will round up to the nearest Gi
	GiWhole := input / Bytes1Gi
	RoundedUpGi := GiWhole + 1
	RoundedUpBytes := RoundedUpGi * Bytes1Gi
	zlog.Debug().Msgf("rounded up to %d Gi which is %d\n", RoundedUpGi, RoundedUpBytes)

	return RoundedUpBytes
}
