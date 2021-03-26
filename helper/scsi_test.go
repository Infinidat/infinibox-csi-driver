package helper

// go test -v ./helper/...

import (
    "testing"
)

const (
    pf string = "set -o pipefail; "
)

// TestYamlBoolToBool tests that YamlBoolToBool() generates bools.
func TestExecScsiCommand(t *testing.T) {
    var tests = []struct {
        cmd string
        want string
        wanterr string
    }{
        {"echo 'foo'", "foo", ""},
        {"true", "", ""},
        {"false", "", "'" + pf + "false' failed"},

        {"[ '1' == '1' ] && echo 'success' || echo 'fail'", "success", ""},
        {"[ '1' == '2' ] && echo 'success' || echo 'fail'", "fail", ""},

        // This would pass without setting pipefail even though grep failed
        // except that ExecScsiCommand() always sets pipefail.
        {"echo 'blah' | grep 'foo' | echo && echo 'success' || echo 'fail'", "fail", ""},
        // Fail with pipefail set
        {"set -o pipefail; echo 'blah' | grep 'foo' | echo && echo 'success' || echo 'fail'", "fail", ""},
    }


    for _, test := range tests {
        answer, err := ExecScsiCommand(test.cmd)
        if !ErrorContains(err, test.wanterr) {
            t.Errorf(`ExecScsiCommand("%s") has err: '%s' != '%s'`, test.cmd, err, test.wanterr)
        }
        if answer != test.want {
            t.Errorf(`ExecScsiCommand("%s") != %s, result: %s`, test.cmd, test.want, answer)
        }
    }
}
