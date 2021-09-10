package helper

import (
	"encoding/json"

	"k8s.io/klog"
)

// Pretty print a struct, map, array or slice variable. Write using klog.V(4).Infof().
func PrettyKlogDebug(msg string, v interface{}) (err error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		klog.V(4).Infof("%s %s", msg, string(b))
	}
	return
}
