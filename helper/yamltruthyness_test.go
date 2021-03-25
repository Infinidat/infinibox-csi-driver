package helper

import (
    "testing"
    "strings"
)

// ErrorContains checks if the error message in out contains the text in
// want.
//
// This is safe when out is nil. Use an empty string for want if you want to
// test that err is nil.
func ErrorContains(out error, want string) bool {
    if out == nil {
        return want == ""
    }
    if want == "" {
        return false
    }
    return strings.Contains(out.Error(), want)
}

func TestYamlBoolToBool(t *testing.T) {
    var tests = []struct {
        input string
        want bool
        wanterr string
    }{
        {"y", true,  ""},
		{"yes", true,  ""},
        {"n", false, ""},
		{"OFF", false, ""},
        {"?", false, "not a valid YAML boolean"},
		{"", false, "not a valid YAML boolean"},
		{"  ", false, "not a valid YAML boolean"},
		{"yesno", false, "not a valid YAML boolean"},
		{"yEs", false, "not a valid YAML boolean"},
		{"nO", false, "not a valid YAML boolean"},
		{"0", false, "not a valid YAML boolean"},
		{"1", false, "not a valid YAML boolean"},
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
