package helper

// go test -v ./helper/...

import (
	"fmt"
	"k8s.io/klog"
	"math/rand"
	"sync"
	"testing"
	"time"
)

const (
	pf string = "set -o pipefail; "
)

// TestExecScsiCommand tests that commands run, errors are handled,
// and may be executed concurrently.
func TestExecScsiCommand(t *testing.T) {
	t.Run("testing that sequential commands execute and errors are returned", func(t *testing.T) {
		execScsi := ExecScsi{}
		var tests = []struct {
			cmd     string
			want    string // Escapes like \\n do not work
			wanterr string
		}{
			{"echo 'foo'", "foo\n", ""},
			{"true", "", ""},
			{"false", "", "'" + pf + "false' failed"},

			{"[ '1' == '1' ] && echo 'success' || echo 'fail'", "success\n", ""},
			{"[ '1' == '2' ] && echo 'success' || echo 'fail'", "fail\n", ""},

			// This would pass, even though grep fails, except that
			// ExecScsiCommand() sets pipefail. Therefore, this correctly fails.
			{"echo 'blah' | grep 'foo' | echo 'force 0' && echo 'success' || echo 'fail'", "force 0\nfail\n", ""},
			// Test line feeds and tabs in output are returned.
			{"echo -e 'foo\nbar\tblah'", "foo\nbar\tblah\n", ""},
		}

		for _, test := range tests {
			answer, err := execScsi.Command(test.cmd)
			if !ErrorContains(err, test.wanterr) {
				t.Errorf(`ExecScsiCommand("%s") has err: '%s' != '%s'`, test.cmd, err, test.wanterr)
			}
			if answer != test.want {
				t.Errorf(`ExecScsiCommand("%s") != %s, result: %s`, test.cmd, test.want, answer)
			}
		}
	})

	t.Run("testing concurrent execution of commands via a goroutine", func(t *testing.T) {
		rand.Seed(time.Now().UnixNano())
		wantedCount := 1000
		sharedFile := "/tmp/testExecScsiCommandConcurrancy"

		execScsi := ExecScsi{}

		var wg sync.WaitGroup
		wg.Add(wantedCount)

		klog.V(4).Infof("here")

		for i := 0; i < wantedCount; i++ {

			go func(w *sync.WaitGroup) {
				klog.V(4).Infof("i: %d", i)
				r := fmt.Sprintf("%d", rand.Int())
				cmd := fmt.Sprintf("echo '%s' > %s && cat %s", r, sharedFile, sharedFile)
				answer, err := execScsi.Command(cmd)
				if err != nil {
					t.Errorf(`ExecScsiCommand("%s") has err: '%s'`, cmd, err)
				}
				if answer != r+"\n" {
					t.Errorf(`ExecScsiCommand("%s") != %s, result: %s`, r, r, answer)
				}
				w.Done()
			}(&wg)
		}
		wg.Wait()
	})
}
