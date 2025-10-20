package helper

/*
Copyright 2024 Infinidat
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

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	leader   string = "lock:"
	follower string = "unlock:"
)

type Exec struct {
	mu sync.Mutex
}

// Command runs commands and may be used with concurrancy.
// All commands will have "set -o pipefail" prepended.
// Parameters:
//
//	cmd - Command to run with pipefail set.
//	args - arguments for the command, can be an empty string
//	isToLogOutput - Optional boolean array. Defaults to allow logging of output. Set to false to suppress logging. Output is always returned.
func (s *Exec) Command(cmd string, args string, isToLogOutput ...bool) (stdOut, stdErr string, err error) {
	s.mu.Lock()
	defer func() {
		stdOut = strings.TrimSpace(stdOut)
		s.mu.Unlock()
		zlog.Trace().Msgf("%s", follower)
	}()

	// Prepend pipefail to cmd
	cmd = strings.TrimSpace(cmd)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", cmd)
	args = strings.TrimSpace(args)
	if args != "" {
		pipefailCmd += " " + args
	}

	zlog.Debug().Msgf("%s %s", leader, pipefailCmd)

	cmdToRun := exec.Command("bash", "-c", pipefailCmd)
	var stdOutBytes, stdErrBytes bytes.Buffer
	cmdToRun.Stdout = &stdOutBytes
	cmdToRun.Stderr = &stdErrBytes
	cmdErr := cmdToRun.Run()
	stdErr = stdErrBytes.String()

	// for now, we will log stderr if it shows up in any command, stderr could show up
	// for errors in commands or even if multipath.conf has deprecation warnings
	if stdErr != "" {
		zlog.Warn().Msgf("command %s stderr [%s]", pipefailCmd, stdErr)
	}
	if cmdErr != nil {
		if nativeError, nativeGetOK := cmdErr.(*exec.ExitError); nativeGetOK {
			var errCode codes.Code
			exitCode := nativeError.ExitCode()
			stdErr = string(nativeError.Stderr)
			// zlog.Debug().Msgf("Command %s had exit code %s", cmd, exitCode)
			if cmd == "iscsiadm" {
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
				err = status.Error(codes.Unknown, fmt.Sprintf("error: %s result: %s", cmdErr, stdOutBytes.String()))
			}
		} else {
			err = status.Error(codes.Unknown, fmt.Sprintf("%s failed with error: %s, result: %s", cmd, cmdErr, stdOutBytes.String()))
		}
		zlog.Error().Msgf("'%s' failed: %s result: %s stderr: %s", pipefailCmd, err, stdOutBytes.String(), stdErrBytes.String())
		return "", stdErr, err
	}

	// Logging is optional, defaults to logged
	if len(isToLogOutput) == 0 || isToLogOutput[0] {
		if len(stdOut) != 0 {
			zlog.Trace().Msgf("Output:\n%s", stdOutBytes.String())
		}
	}

	return stdOutBytes.String(), stdErrBytes.String(), nil
}
