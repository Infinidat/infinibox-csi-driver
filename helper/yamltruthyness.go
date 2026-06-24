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
	"errors"
	"fmt"
	"slices"
	"strings"
)

func getYamlBoolsFalse() []string {
	bools := strings.Split("n|N|no|No|NO|false|False|FALSE|off|Off|OFF", "|")
	return bools
}

func getYamlBoolsTrue() []string {
	bools := strings.Split("y|Y|yes|Yes|YES|true|True|TRUE|on|On|ON", "|")
	return bools
}

// Many strings are true in YAML. Convert to boolean.
// Ref: https://yaml.org/type/bool.html
func YamlBoolToBool(boolString string) (bool, error) {
	if Contains(getYamlBoolsTrue(), boolString) {
		return true, nil
	}
	if Contains(getYamlBoolsFalse(), boolString) {
		return false, nil
	}
	msg := fmt.Sprintf("'%s' is not a valid YAML boolean", boolString)
	fmt.Print(msg)
	return false, errors.New(msg)
}

// Contains tells whether 'x' is found within the array of strings 'a'.
func Contains(arrayOfStrings []string, substring string) bool {
	return slices.Contains(arrayOfStrings, substring)
}
