package helper

// Ref: https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/sync

import (
	"errors"
	"fmt"
	"k8s.io/klog"
	"os/exec"
	"sync"
)

const (
	leader   string = "LOCK:"
	follower string = "UNLOCK:"
)

type ExecScsi struct {
	mu sync.Mutex
}

// Command runs commands and may be used with concurrancy.
// All commands will have "set -o pipefail" prepended.
// Parameters:
//   cmd - Command to run with pipefail set.
//   isToLogOutput - Optional boolean array. Defaults to allow logging of output. Set to false to suppress logging. Output is always returned.
func (s *ExecScsi) Command(cmd string, isToLogOutput ...bool) (out string, err error) {
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		klog.V(4).Infof("%s", follower)
	}()

	var result []byte

	// Prepend pipefail to cmd
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", cmd)

	klog.V(4).Infof("%s %s", leader, pipefailCmd)

	result, err = exec.Command("bash", "-c", pipefailCmd).CombinedOutput()

	if err != nil {
		msg := fmt.Sprintf("'%s' failed: %s", pipefailCmd, err)
		klog.Errorf(msg)
		return "", errors.New(msg)
	}

	out = string(result)

	// Logging is optional, defaults to logged
	if len(isToLogOutput) == 0 || isToLogOutput[0] {
		if len(out) != 0 {
			klog.V(4).Infof("Output: %s", out)
		}
	}

	return out, nil
}
