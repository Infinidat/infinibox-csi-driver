package e2e

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func VerifyDirPermsCorrect(clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string,
	expectedValue string) (bool, string, error) {

	command := fmt.Sprintf("ls -ld %s", MOUNT_PATH)

	stdOut, stdErr, err := execCmdInPod(clientSet, config, podName, nameSpace, command)

	if err != nil {
		fmt.Printf("Error happened attempting to exec command in pod: %s\n", err.Error())
		return false, "", err
	}

	output := strings.Fields(stdOut)

	var actualValue string
	if len(output) == 0 {
		actualValue = "stdout was empty"
	} else {
		actualValue = strings.TrimSpace(output[0])
	}

	fmt.Printf("Expected: %s\n", expectedValue)
	fmt.Printf("Actual: %s\n", actualValue)
	//fmt.Printf("Output: %s\n", stdOut) // uncomment this to get un-parsed command output

	if len(stdErr) > 0 {
		fmt.Printf("Error: %s\n", stdErr)
	}

	return (math.Abs(float64(strings.Compare(actualValue, expectedValue)))) == 0, actualValue, nil

}

func VerifyGroupIdIsUsed(clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string,
	expectedValue string) (bool, string, error) {

	createFileCmd := fmt.Sprintf("touch %s/testfile.txt", MOUNT_PATH)
	testFileCmd := fmt.Sprintf("ls -l %s/testfile.txt", MOUNT_PATH)

	_, _, err := execCmdInPod(clientSet, config, podName, nameSpace, createFileCmd)

	if err != nil {
		fmt.Println("Error happened creating test file")
		return false, "", err
	}

	stdOut, stdErr, err := execCmdInPod(clientSet, config, podName, nameSpace, testFileCmd)

	if err != nil {
		fmt.Println("Error happened reading test file")
		return false, "", err
	}

	output := strings.Fields(stdOut)

	actualValue := strings.TrimSpace(output[3])

	fmt.Printf("Expected:%s\n", expectedValue)
	fmt.Printf("Actual: %s\n", actualValue)
	//fmt.Printf("Output: %s\n", stdOut) // uncomment if you want unparsed output

	if len(stdErr) > 0 {
		fmt.Printf("Error: %s\n", stdErr)
	}
	return (math.Abs(float64(strings.Compare(actualValue, expectedValue)))) == 0, actualValue, nil

}

func VerifyBlockWriteInPod(clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string) (bool, string, error) {

	fmt.Printf("Testing for blockwrite in %s\n", podName)

	nodeNameCmd := "echo $KUBE_NODE_NAME"

	nodeName, stdErr, err := execCmdInPod(clientSet, config, podName, nameSpace, nodeNameCmd)

	if len(stdErr) > 0 {
		fmt.Printf("Error: %s\n", stdErr)
	}

	if err != nil {
		fmt.Printf("Error happened attempting to exec command in pod: %s\n", err.Error())
		return false, err.Error(), err
	}

	charCount := 0 // default to read no characters

	if len(nodeName) > 0 {
		charCount = len(strings.Fields(nodeName)[0])
	}

	// fmt.Printf("Nodename length is %d\n", charCount)

	testFileCmd := fmt.Sprintf("dd count=%d if=%s ibs=1 2>/dev/null", charCount, BLOCK_DEV_PATH)

	// fmt.Printf("Character count is: %d and testCmd: %s\n", charCount, testFileCmd)

	blockRead, stdErr2, err2 := execCmdInPod(clientSet, config, podName, nameSpace, testFileCmd)

	if len(stdErr2) > 0 {
		fmt.Printf("Error: %s\n", stdErr2)
	}

	if err2 != nil {
		fmt.Printf("Error happened attempting to exec command in pod: %s\n", err2.Error())
		return false, err2.Error(), err
	}

	// fmt.Printf("Result from reading pod is: %s\n", blockRead)

	if strings.TrimSpace(nodeName) == strings.TrimSpace(blockRead) {
		return true, "", nil
	}
	return false, "Hostname did not match block written and read.", nil
}

// execCmdInPod - exec command on specific pod and wait the command's output.
func execCmdInPod(clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string,
	command string) (string, string, error) {

	stdOut := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}

	cmd := []string{
		"/bin/sh",
		"-c",
		command,
	}
	req := clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(nameSpace).
		SubResource("exec")
	// need container name?

	req.VersionedParams(
		&v1.PodExecOptions{
			Command: cmd,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     true,
		},
		scheme.ParameterCodec,
	)

	//fmt.Printf("execCmdInPod - Running command: %s\n", command)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return stdOut.String(), stdErr.String(), err
	}
	err = exec.StreamWithContext(context.Background(),
		remotecommand.StreamOptions{
			Stdin:  nil,
			Stdout: stdOut,
			Stderr: stdErr,
		})

	return stdOut.String(), stdErr.String(), err
}
