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
	"fmt"
	"regexp"
	"runtime"
	"time"

	"github.com/rs/zerolog"
)

/**
func SomeFunction(list *[]string) {
    defer TimeTrack(time.Now())
    // Do whatever you want.
}
*/

func TimeTrack(zlog zerolog.Logger, start time.Time) {
	elapsed := time.Since(start)

	// Skip this function, and fetch the PC and file for its parent.
	pc, _, _, _ := runtime.Caller(1)

	// Retrieve a function object this functions parent.
	funcObj := runtime.FuncForPC(pc)

	// Regex to extract just the function name (and not the module path).
	runtimeFunc := regexp.MustCompile(`^.*\.(.*)$`)
	name := runtimeFunc.ReplaceAllString(funcObj.Name(), "$1")

	zlog.Debug().Msgf(fmt.Sprintf("%s took %s", name, elapsed.Round(1*time.Millisecond)))
}

func TimeTrackBasic(zlog zerolog.Logger, start time.Time, msg string) {
	elapsed := time.Since(start)

	zlog.Debug().Msgf(fmt.Sprintf("%s took %s", msg, elapsed.Round(1*time.Millisecond)))
}
