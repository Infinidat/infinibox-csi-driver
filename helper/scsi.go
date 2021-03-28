package helper

// Ref: https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/sync

import (
	"errors"
	"fmt"
	"k8s.io/klog"
	"os/exec"
	"strings"
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
func (s *ExecScsi) Command(cmd string) (out string, err error) {
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		klog.V(4).Infof("%s", follower)
	}()

	var result []byte

	// Prepend pipefail to cmd
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", cmd)

	klog.V(4).Infof("%s %s", leader, pipefailCmd)

	result, err = exec.Command("bash", "-c", pipefailCmd).Output()
	out = strings.Replace(string(result), "\n", "", -1)

	if err != nil {
		msg := fmt.Sprintf("'%s' failed", pipefailCmd)
		klog.Errorf(msg)
		return "", errors.New(msg)
	}

	if len(out) != 0 {
		klog.V(4).Infof("out: %s", out)
	}
	return out, nil
}
