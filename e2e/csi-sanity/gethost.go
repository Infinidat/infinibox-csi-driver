package main

import (
	"fmt"
	"infinibox-csi-driver/e2e"
	"os"
)

func main() {
	hostName, err := e2e.GetKubeHost()
	if err != nil {
		fmt.Printf("error getting kube host %s\n", err.Error())
		os.Exit(2)
	}
	fmt.Printf("%s", hostName)
}
