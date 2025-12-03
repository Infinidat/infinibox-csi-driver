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
