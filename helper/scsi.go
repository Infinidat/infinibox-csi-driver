package helper

import (
	"errors"
	"fmt"
    "k8s.io/klog"
    "os/exec"
    "strings"
    "sync"
)

const (
    leader string = "Run:"
)

type ExecScsi struct {
    mu sync.Mutex
}

func (s *ExecScsi) Command(cmd string) (out string, err error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    var result []byte

    pipefailCmd := fmt.Sprintf("set -o pipefail; %s", cmd)

    klog.V(4).Infof("%s %s", leader, pipefailCmd)
	result, err = exec.Command("bash", "-c", pipefailCmd).Output()
    out = strings.Replace(string(result), "\n", "", -1)

	if err != nil {
        msg := fmt.Sprintf("'%s' failed", pipefailCmd)
        klog.Errorf(msg)
        return "", errors.New(msg)
	}

    klog.V(4).Infof("out: %s", out)
    return out, nil 
}
