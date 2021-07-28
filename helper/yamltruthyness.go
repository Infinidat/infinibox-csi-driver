package helper

import (
	"errors"
	"fmt"
	"k8s.io/klog"
	"strings"
)

func getYamlBoolsAll() (bools []string) {
	bools = strings.Split("y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF", "|")
	return bools
}

func getYamlBoolsFalse() (bools []string) {
	bools = strings.Split("n|N|no|No|NO|false|False|FALSE|off|Off|OFF", "|")
	return bools
}

func getYamlBoolsTrue() (bools []string) {
	bools = strings.Split("y|Y|yes|Yes|YES|true|True|TRUE|on|On|ON", "|")
	return bools
}

// Many strings are true in YAML. Convert to boolean.
// Ref: https://yaml.org/type/bool.html
func YamlBoolToBool(b string) (myBool bool, err error) {
	if Contains(getYamlBoolsTrue(), b) {
		return true, nil
	} else if Contains(getYamlBoolsAll(), b) {
		return false, nil
	}
	msg := fmt.Sprintf("'%s' is not a valid YAML boolean", b)
	klog.Errorf(msg)
	return false, errors.New(msg)
}

// Contains tells whether 'x' is found within the array of strings 'a'.
func Contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}
