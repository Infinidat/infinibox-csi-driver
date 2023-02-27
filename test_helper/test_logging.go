package test_helper

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/klog/v2"
)

func ConfigureKlog() {
	// Set klog log level to 99. Use in UTs.
	// By default klog will only write V(0) messages.
	logLevel := "99"
	fmt.Printf("Configuring KLOG V level: %s\n", logLevel)
	fs := flag.FlagSet{}
	klog.InitFlags(&fs)
	err := fs.Set("v", logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set klog verbosity: %s\n", logLevel)
		os.Exit(3)
	}
}
