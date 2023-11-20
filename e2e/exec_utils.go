package e2e

import (
	"bytes"
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"math"
	"strings"
)

func VerifyDirPermsCorrect(clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string,
	expectedValue string) (bool, string) {

	command := fmt.Sprintf("ls -ld %s", MOUNT_PATH)

	stdOut, stdErr, err := execCmdInPod(clientSet, config, podName, nameSpace, command)

	if err != nil {
		fmt.Printf("Error happened attempting to exec command in pod: %s\n", err.Error())
	}

	output := strings.Fields(stdOut)

	actualValue := strings.TrimSpace(output[0])

	fmt.Printf("Expected: %s\n", expectedValue)
	fmt.Printf("Actual: %s\n", actualValue)
	//fmt.Printf("Output: %s\n", stdOut) // uncomment this to get un-parsed command output

	if len(stdErr) > 0 {
		fmt.Printf("Error: %s\n", stdErr)
	}

	return (math.Abs(float64(strings.Compare(actualValue, expectedValue)))) == 0, actualValue

}

func VerifyGroupIdIsUsed(clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string,
	expectedValue string) (bool, string) {

	createFileCmd := fmt.Sprintf("touch %s/testfile.txt", MOUNT_PATH)
	testFileCmd := fmt.Sprintf("ls -l %s/testfile.txt", MOUNT_PATH)

	_, _, err := execCmdInPod(clientSet, config, podName, nameSpace, createFileCmd)

	if err != nil {
		fmt.Println("Error happened creating test file")
	}

	stdOut, stdErr, err := execCmdInPod(clientSet, config, podName, nameSpace, testFileCmd)

	if err != nil {
		fmt.Println("Error happened reading test file")
	}

	output := strings.Fields(stdOut)

	actualValue := strings.TrimSpace(output[3])

	fmt.Printf("Expected:%s\n", expectedValue)
	fmt.Printf("Actual: %s\n", actualValue)
	//fmt.Printf("Output: %s\n", stdOut) // uncomment if you want unparsed output

	if len(stdErr) > 0 {
		fmt.Printf("Error: %s\n", stdErr)
	}
	return (math.Abs(float64(strings.Compare(actualValue, expectedValue)))) == 0, actualValue

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

	//fmt.Printf("Running command: %s\n", command)

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
