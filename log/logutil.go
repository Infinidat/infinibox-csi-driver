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
	"fmt"
	"os"
	"strings"
	"time"

	"sync"

	"github.com/rs/zerolog"
)

var once sync.Once
var logLevel zerolog.Level
var logger zerolog.Logger

// formats the logger to output a standard time format (RFC3339) and format output to
// a readable/parsable output. For good zerolog tutorial
// see: https://betterstack.com/community/guides/logging/zerolog/
func Get() zerolog.Logger {

	once.Do(func() {
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

		output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339} // 2023-07-11T14:54:44Z

		output.FormatLevel = func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
		}
		output.FormatMessage = func(i interface{}) string {
			return fmt.Sprintf("%s", i) // modify this line to adjust the actual message output
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
			Timestamp().
			Logger()
	})

	return logger
}
