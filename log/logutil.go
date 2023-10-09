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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sync"

	"github.com/rs/zerolog"
	"k8s.io/klog/v2"
)

var once sync.Once
var logLevel zerolog.Level
var logger zerolog.Logger

// formats the logger to output a standard time format (RFC3339) and format output to
// a readable/parsable output. For good zerolog tutorial
// see: https://betterstack.com/community/guides/logging/zerolog/
func Get() zerolog.Logger {

	once.Do(func() {

		zerolog.CallerMarshalFunc = shortFileFormat

		appLogLevel := os.Getenv("APP_LOG_LEVEL")
		switch appLogLevel {
		case "quiet":
			logLevel = zerolog.Disabled
		case "error":
			logLevel = zerolog.ErrorLevel
		case "warn":
			logLevel = zerolog.WarnLevel
		case "info":
			logLevel = zerolog.InfoLevel
		case "debug":
			logLevel = zerolog.DebugLevel
		case "trace":
			logLevel = zerolog.TraceLevel
		default:
			logLevel = zerolog.InfoLevel
		}

		zerolog.TimeFieldFormat = time.RFC3339Nano

		output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339Nano} // 2023-07-11T14:54:44Z

		output.FormatLevel = func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
		}
		output.FormatMessage = func(i interface{}) string {
			return fmt.Sprintf("%s", i) // modify this line to adjust the actual message output
		}
		output.FormatCaller = func(i interface{}) string {
			return filepath.Base(fmt.Sprintf("%s", i))
		}
		output.FormatFieldName = func(i interface{}) string {
			return fmt.Sprintf("%s:", i)
		}
		output.FormatFieldValue = func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("%s", i))
		}
		logger = zerolog.New(output).
			Level(zerolog.Level(logLevel)).
			With().
			Caller(). // calling file line #
			Timestamp().
			Logger()
	})

	return logger
}

// used to format calling file line to shorter version.
func shortFileFormat(pc uintptr, file string, line int) string {
	short := file
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			short = file[i+1:]
			break
		}
	}
	file = short
	return file + ":" + strconv.Itoa(line)
}

func SetupKlog() {
	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("stderrthreshold", "WARNING")
	var verbosity string
	appLogLevel := os.Getenv("APP_LOG_LEVEL")
	switch appLogLevel {
	case "quiet":
		verbosity = "1"
	case "info":
		verbosity = "2"
	case "extended":
		verbosity = "3"
	case "debug":
		verbosity = "4"
	case "trace":
		verbosity = "5"
	default:
		verbosity = "2"
	}
	_ = flag.Set("v", verbosity)
	flag.Parse()
}

// CheckForLogLevelOverride looks for a file on the node's file system that would hold a log level, if found, it will
// use that log level instead of the normally set log level, this is useful for debugging a specific
// node in a production setting where there are possibly many nodes and you only want debug level logging for
// a specific node, to use it, create the file on the node, then restart the node's Pod
func CheckForLogLevelOverride() {
	const LOGLEVEL_FILE = "/host/etc/infinidat-csi-loglevel"
	buf, err := os.ReadFile(LOGLEVEL_FILE)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		fmt.Printf("error reading %s %s\n", LOGLEVEL_FILE, err.Error())
		return
	}
	logLevel := strings.TrimSpace(string(buf))
	err = os.Setenv("APP_LOG_LEVEL", logLevel)
	if err != nil {
		fmt.Printf("error setting APP_LOG_LEVEL env var %s\n", err.Error())
	}
	fmt.Printf("overriding log level from %s with [%s]\n", LOGLEVEL_FILE, logLevel)
}
