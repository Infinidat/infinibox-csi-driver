package helper

// Ref: https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/sync

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

const (
	leader   string = "lock:"
	follower string = "unlock:"
)

type ExecScsi struct {
	mu sync.Mutex
}

// Command runs commands and may be used with concurrancy.
// All commands will have "set -o pipefail" prepended.
// Parameters:
//   cmd - Command to run with pipefail set.
//   args - arguments for the command, can be an empty string
//   isToLogOutput - Optional boolean array. Defaults to allow logging of output. Set to false to suppress logging. Output is always returned.
func (s *ExecScsi) Command(cmd string, args string, isToLogOutput ...bool) (out string, err error) {
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		klog.V(4).Infof("%s", follower)
	}()

	var result []byte

	// Prepend pipefail to cmd
	cmd = strings.TrimSpace(cmd)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", cmd)
	args = strings.TrimSpace(args)
	if args != "" {
		pipefailCmd += " " + args
	}

	klog.V(4).Infof("%s %s", leader, pipefailCmd)

	result, cmdErr := exec.Command("bash", "-c", pipefailCmd).CombinedOutput()

	if cmdErr != nil {
		if cmd == "iscsiadm" {
			if nativeError, nativeGetOK := cmdErr.(*exec.ExitError); nativeGetOK {
				var errCode codes.Code
				exitCode := nativeError.ExitCode()
				switch exitCode {
				case 2:
					errCode = codes.NotFound // Session not found
				case 13:
					errCode = codes.PermissionDenied // no OS permissions to access iscsid or execute iscsiadm
				case 15:
					errCode = codes.AlreadyExists // no OS permissions to access iscsid or execute iscsiadm
				case 24:
					errCode = codes.Unauthenticated // login failed due to authorization failure
				default:
					errCode = codes.Internal // all other errors may be considered internal
				}
				err = status.Error(errCode, fmt.Sprintf("iscsiadm error: %d, %s", exitCode, cmdErr))
			} else {
				err = status.Error(codes.Unknown, fmt.Sprintf("iscsiadm error: unknown, %s", cmdErr))
			}
		} else {
			err = status.Error(codes.Unknown, fmt.Sprintf("%s error, %s", cmd, cmdErr))
		}
		klog.Errorf("'%s' failed: %s", pipefailCmd, err)
		return "", err
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
