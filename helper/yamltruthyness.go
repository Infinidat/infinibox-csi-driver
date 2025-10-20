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
