package helper

// go test -v ./helper/...
// go test ./helper -run TestExecScsiCommand

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"k8s.io/klog"
)

// const (
// 	pf string = "set -o pipefail; "
// )

// TestExecScsiCommand tests that commands run, errors are handled,
// and may be executed concurrently.
func TestExecScsiCommand(t *testing.T) {
	t.Run("testing that sequential commands execute and errors are returned", func(t *testing.T) {
		execScsi := ExecScsi{}
		tests := []struct {
			cmd     string
			args    string
			want    string // Escapes like \\n do not work
			wanterr string
		}{
			{"echo", "foo", "foo\n", ""},
			{"true", "", "", ""},
			{"false", "", "", "exit status 1"},

			{"bash", "-c \"[ '1' == '1' ] && echo 'success' || echo 'fail'\"", "success\n", ""},
			{"bash", "-c \"[ '1' == '2' ] && echo 'success' || echo 'fail'\"", "fail\n", ""},

			// This would pass, even though grep fails, except that
			// ExecScsiCommand() sets pipefail. Therefore, this correctly fails.
			{"echo", "'blah' | grep 'foo' | echo 'force 0' && echo 'success' || echo 'fail'", "force 0\nfail\n", ""},
			// Test line feeds and tabs in output are returned.
			{"echo", "-e 'foo\nbar\tblah'", "foo\nbar\tblah\n", ""},

			// Test failure with writing to stderr
			{"echo", "stderr >&2", "stderr\n", ""},
			{"echo", "stdout; >&2 echo stderr", "stdout\nstderr\n", ""},
		}

		for _, test := range tests {
			answer, err := execScsi.Command(test.cmd, test.args)
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

		var i int
		for i = 0; i < wantedCount; i++ {
			go func(w *sync.WaitGroup, i int) {
				klog.V(4).Infof("i: %d", i)
				r := fmt.Sprintf("%d", rand.Int())
				cmd := "echo"
				args := fmt.Sprintf("'%s' > %s && cat %s", r, sharedFile, sharedFile)
				answer, err := execScsi.Command(cmd, args)
				if err != nil {
					t.Errorf(`ExecScsiCommand("%s") has err: '%s'`, cmd, err)
				}
				if answer != r+"\n" {
					t.Errorf(`ExecScsiCommand("%s") != %s, result: %s`, r, r, answer)
				}
				w.Done()
			}(&wg, i)
		}
		wg.Wait()
	})
}
