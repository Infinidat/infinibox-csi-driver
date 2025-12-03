/*
Copyright 2022 Infinidat
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

package log

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"k8s.io/klog/v2"
)

func SetupKlog() {
	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("stderrthreshold", "WARNING")
	appLogLevel := os.Getenv("APP_LOG_LEVEL")

	switch appLogLevel {
	case "quiet":
		_ = flag.Set("v", "1")
	case "info":
		_ = flag.Set("v", "2")
	case "extended":
		_ = flag.Set("v", "3")
	case "debug":
		_ = flag.Set("v", "4")
	case "trace":
		_ = flag.Set("v", "5")
	default:
		_ = flag.Set("v", "2")
	}
	flag.Parse()
}

// CheckForLogLevelOverride looks for a file on the node's file system that would hold a log level, if found, it will
// use that log level instead of the normally set log level, this is useful for debugging a specific
// node in a production setting where there are possibly many nodes and you only want debug level logging for
// a specific node, to use it, create the file on the node, then restart the node's Pod
func CheckForLogLevelOverride() {
	const logLevelFile = "/host/etc/infinidat-csi-loglevel"
	buf, err := os.ReadFile(logLevelFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		fmt.Printf("error reading %s %s\n", logLevelFile, err.Error())
		return
	}
	logLevel := strings.TrimSpace(string(buf))
	err = os.Setenv("APP_LOG_LEVEL", logLevel)
	if err != nil {
		fmt.Printf("error setting APP_LOG_LEVEL env var %s\n", err.Error())
	}
	fmt.Printf("overriding log level from %s with [%s]\n", logLevelFile, logLevel)
}
