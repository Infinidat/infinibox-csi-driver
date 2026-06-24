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

// go test -v ./helper/...
// go test ./helper -run TestExecScsiCommand

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
)

// const (
// 	pf string = "set -o pipefail; "
// )

// TestExecCommand tests that commands run, errors are handled,
// and may be executed concurrently.
func TestExecCommand(t *testing.T) {
	t.Run("testing that sequential commands execute and errors are returned", func(t *testing.T) {
		exec := Exec{}
		tests := []struct {
			cmd     string
			args    string
			want    string // Escapes like \\n do not work
			wanterr string
		}{
			{"echo", "foo", "foo", ""},
			{"true", "", "", ""},
			{"false", "", "", "exit status 1"},

			{"bash", "-c \"[ '1' == '1' ] && echo 'success' || echo 'fail'\"", "success", ""},
			{"bash", "-c \"[ '1' == '2' ] && echo 'success' || echo 'fail'\"", "fail", ""},

			// This would pass, even though grep fails, except that
			// ExecCommand() sets pipefail. Therefore, this correctly fails.
			{"echo", "'blah' | grep 'foo' | echo 'force 0' && echo 'success' || echo 'fail'", "force 0\nfail", ""},
			// Test line feeds and tabs in output are returned.
			{"echo", "-e 'foo\nbar\tblah'", "foo\nbar\tblah", ""},

			// test that stderr is not being combined into stdout
			{"echo", "stderr >&2", "", ""},
		}

		for _, test := range tests {
			answer, _, err := exec.Command(test.cmd, test.args)
			if !ErrorContains(err, test.wanterr) {
				t.Errorf(`ExecCommand("%s") has err: '%s' != '%s'`, test.cmd, err, test.wanterr)
			}
			if answer != test.want {
				t.Errorf(`ExecCommand("%s") != %s, result: %s`, test.cmd, test.want, answer)
			}
		}
	})

	t.Run("testing concurrent execution of commands via a goroutine", func(t *testing.T) {
		wantedCount := 1000
		sharedFile := "/tmp/testExecCommandConcurrancy"

		execScsi := Exec{}

		var wg sync.WaitGroup
		wg.Add(wantedCount)

		var i int
		for i = 0; i < wantedCount; i++ {
			go func(w *sync.WaitGroup, i int) {
				fmt.Printf("i: %d", i)
				r := fmt.Sprintf("%d", rand.Int())
				cmd := "echo"
				args := fmt.Sprintf("'%s' > %s && cat %s", r, sharedFile, sharedFile)
				answer, _, err := execScsi.Command(cmd, args)
				if err != nil {
					t.Errorf(`ExecCommand("%s") has err: '%s'`, cmd, err)
				}
				if answer != r {
					t.Errorf(`ExecCommand("%s") != %s, result: %s`, r, r, answer)
				}
				w.Done()
			}(&wg, i)
		}
		wg.Wait()
	})
}
