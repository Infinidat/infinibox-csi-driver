package helper

import (
	"errors"
	"fmt"
    "k8s.io/klog"
    "os/exec"
    "strings"
)

const (
    leader string = "Run:"
)

func ExecScsiCommand(cmd string) (out string, err error) {
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
