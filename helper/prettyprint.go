package helper

import (
	"encoding/json"

	"fmt"
	"k8s.io/klog"
)

// Pretty print a struct, map, array or slice variable. Write using klog.V(4).Infof().
func PrettyKlogDebug(msg string, v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		klog.V(4).Infof("%s %s", msg, string(b))
	} else {
		msg := fmt.Sprintf("Failed to pretty print. Falling back to print. Message: %s. Var: %+v. Error: %+v.", msg, v, err)
		klog.Errorf(msg)
	}
}
