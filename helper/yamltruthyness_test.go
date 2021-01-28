package helper

import (
    "testing"
    "strings"
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
    var tests = []struct {
        input string
        want bool
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
