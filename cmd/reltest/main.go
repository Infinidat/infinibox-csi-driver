package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

/**
example usage:
go run ./cmd/reltest/main.go -openshift=false -configs=/home/jeffmc/k8screds -clusters csi-test,csidev-rhel97 e2enfsanno e2enfs e2eiscsi

the code depends on kubeconfig files being in a certain directory with certain naming convention
	example: csi-osv.kubeconfig
*/

// runMakefileTarget invokes the 'make' command for a given target.
func runMakefileTarget(target string) error {
	// Initialize the system command: make <target>
	cmd := exec.Command("make", target)

	// Stream stdout and stderr directly to the terminal
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Optional: Bind stdin if your Makefile targets are interactive
	cmd.Stdin = os.Stdin

	fmt.Printf("🚀 Executing Makefile target: '%s'...\n", target)

	// Run the command and wait for its completion
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute target %s: %w", target, err)
	}

	return nil
}

const (
	FAIL    = "fail"
	SUCCESS = "success"
)

var clusterSummary map[string]map[string]string
var summary map[string]string
var requested []string

func main() {
	clusterList := flag.String("clusters", "", "comma separated list of cluster names to test against")
	configsDir := flag.String("configs", "", "configs directory, full path")
	openshift := flag.Bool("openshift", false, "openshift")

	flag.Parse()
	requested = flag.Args()
	if *configsDir == "" {
		fmt.Printf("🔥 Error: no configs listed on command line flag\n")
		printUsage()
		os.Exit(1)
	}
	fmt.Printf("clusters to test +%v\n", *clusterList)
	if *clusterList == "" {
		fmt.Printf("🔥 Error: no clusters listed on command line flag\n")
		printUsage()
		os.Exit(1)
	}
	fmt.Printf("targets requested +%v\n", requested)

	summary = make(map[string]string)
	clusterSummary = make(map[string]map[string]string)

	clusters := strings.Split(*clusterList, ",")
	for _, cluster := range clusters {
		verifyClusterKubeconfig(*configsDir, cluster)
	}

	for _, cluster := range clusters {
		setClusterKubeconfig(*configsDir, cluster, *openshift)
		// Execute the target
		for _, target := range requested {
			if err := runMakefileTarget(target); err != nil {
				var err error
				fmt.Printf("❌ Error: %v cluster: %s\n", err, cluster)
				summary[target] = FAIL
			} else {
				fmt.Printf("✅ Target %s on cluster %s executed successfully!\n", target, cluster)
				summary[target] = SUCCESS
			}
		}
		clusterSummary[cluster] = summary
	}
	printSummary()
}

func printSummary() {

	fmt.Printf("\n--------------------------------------------------------------------\n")
	fmt.Printf("\nSummary of Testing\n")
	for clusterName, cluster := range clusterSummary {
		for _, target := range requested {
			s := cluster[target]
			switch s {
			case SUCCESS:
				fmt.Printf("✅ Target %s Cluster %s executed successfully!\n", clusterName, target)
			case FAIL:
				fmt.Printf("❌ Target %s Cluster %s failed!\n", clusterName, target)
			}
		}
	}

}

func printUsage() {
	fmt.Printf("usage:  -configs /home/foo/configs -clusters cluster1,cluster2 target1 target2\n")
}

func setClusterKubeconfig(configDir string, clusterName string, openshift bool) {
	pathName := configDir + "/" + clusterName + ".kubeconfig"
	err := os.Setenv("KUBECONFIG", pathName)
	if err != nil {
		fmt.Printf("error in setenv %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("setenv KUBECONFIG=%s\n", pathName)
	if openshift {
		openshiftLogin(configDir, clusterName)
	}
}

func verifyClusterKubeconfig(configDir string, clusterName string) {
	pathName := configDir + "/" + clusterName + ".kubeconfig"
	_, err := os.Stat(pathName)
	if err != nil {
		fmt.Printf("🔥 error in finding kubeconfig file %s %s\n", pathName, err.Error())
		os.Exit(1)
	}
	fmt.Printf("ℹ️ kubeconfig '%s' exists...\n", pathName)
}

func openshiftLogin(configDir string, clusterName string) {
	//       oc login https://$1:6443 -u kubeadmin -p `cat ~/k8screds/$1.kubeadmin.password`
	url := "https://" + clusterName + ":6443"
	buff, err := os.ReadFile(configDir + "/" + clusterName + ".kubeadmin.password")
	if err != nil {
		fmt.Printf("🔥 error reading openshift password file %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("password is [%s]\n", strings.TrimSpace(string(buff)))
	cmd := exec.Command("oc", "login", url, "-u", "kubeadmin", "-p", strings.TrimSpace(string(buff)))

	// Stream stdout and stderr directly to the terminal
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command and wait for its completion
	if err := cmd.Run(); err != nil {
		fmt.Printf("🔥 error in openshift login to %s - error %s\n", clusterName, err.Error())
		os.Exit(1)
	}
	fmt.Printf("ℹ️ openshift login to: '%s' successful...\n", clusterName)
}
