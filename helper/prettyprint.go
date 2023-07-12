package helper

import (
	"encoding/json"

	"fmt"
)

// Pretty print a struct, map, array or slice variable.
func PrettyKlogDebug(msg string, v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		zlog.Info().Msgf("%s %s", msg, string(b))
	} else {
		msg := fmt.Sprintf("Failed to pretty print. Falling back to print. Message: %s. Var: %+v. Error: %+v.", msg, v, err)
		zlog.Error().Msgf(msg)
	}
}
