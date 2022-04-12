/*Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/

package logger

import (
	"context"

	csictx "github.com/rexray/gocsi/context"
	"github.com/sirupsen/logrus"
)

var (
	logInstance *logrus.Logger
	logLevel    string
)

type Fields map[string]interface{}

func getLoggerInstance() *logrus.Logger {
	if logInstance == nil {
		logInstance = logrus.New()
		// logInstance.SetReportCaller(true)
		logLevel, _ = csictx.LookupEnv(context.Background(), "APP_LOG_LEVEL")

		// set global log level
		ll, err := logrus.ParseLevel(logLevel)
		if err != nil {
			logrus.Error("Invalid logging level: ", logLevel)
			ll = logrus.InfoLevel
		}
		logInstance.SetLevel(ll)
		logrus.Info("Log level set to ", logInstance.GetLevel().String())

	}
	return logInstance
}

func GetLevel() string {
	return getLoggerInstance().GetLevel().String()
}

func Trace(args ...interface{}) {
	getLoggerInstance().Trace(args...)
}

func Traceln(args ...interface{}) {
	getLoggerInstance().Traceln(args...)
}

func Tracef(format string, args ...interface{}) {
	getLoggerInstance().Tracef(format, args...)
}

func Debug(args ...interface{}) {
	getLoggerInstance().Debug(args...)
}

func Debugln(args ...interface{}) {
	getLoggerInstance().Debugln(args...)
}

func Debugf(format string, args ...interface{}) {
	getLoggerInstance().Debugf(format, args...)
}

func WithField(key string, value interface{}) *logrus.Entry {
	return getLoggerInstance().WithField(key, value)
}

func WithFields(fields Fields) *logrus.Entry {
	lfileds := logrus.Fields{}
	for k, v := range fields {
		lfileds[k] = v
	}

	return getLoggerInstance().WithFields(lfileds)
}

func Info(args ...interface{}) {
	getLoggerInstance().Info(args...)
}

func Infoln(args ...interface{}) {
	getLoggerInstance().Infoln(args...)
}

func Infof(format string, args ...interface{}) {
	getLoggerInstance().Infof(format, args...)
}

func Warn(args ...interface{}) {
	getLoggerInstance().Warn(args...)
}

func Warnln(args ...interface{}) {
	getLoggerInstance().Warnln(args...)
}

func Warnf(format string, args ...interface{}) {
	getLoggerInstance().Warnf(format, args...)
}

func Error(args ...interface{}) {
	getLoggerInstance().Error(args...)
}

func Errorln(args ...interface{}) {
	getLoggerInstance().Errorln(args...)
}

func Errorf(format string, args ...interface{}) {
	getLoggerInstance().Errorf(format, args...)
}

func Fatal(args ...interface{}) {
	getLoggerInstance().Fatal(args...)
}

func Fatalln(args ...interface{}) {
	getLoggerInstance().Fatalln(args...)
}

func Fatalf(format string, args ...interface{}) {
	getLoggerInstance().Fatalf(format, args...)
}

func Panic(args ...interface{}) {
	getLoggerInstance().Panic(args...)
}

func Panicln(args ...interface{}) {
	getLoggerInstance().Panicln(args...)
}

func Panicf(format string, args ...interface{}) {
	getLoggerInstance().Panicf(format, args...)
}
