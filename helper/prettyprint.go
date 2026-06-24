/*
Copyright 2026 Infinidat
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

package helper

import (
	"encoding/json"
	"log/slog"

	"fmt"
)

// Pretty print a struct, map, array or slice variable.
func PrettyKlogDebug(msg string, v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		slog.Debug("info", "message", msg, "indented", string(b))
	} else {
		msg := fmt.Sprintf("Failed to pretty print. Falling back to print. Message: %s. Var: %+v. Error: %+v.", msg, v, err)
		slog.Error(msg)
	}
}
