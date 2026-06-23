package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	sc "github.com/infinidat/infinibox-csi-driver/storage/common"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func VerifyDirPermsCorrect(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string, expectedValue string) (bool, string, error) {
	time.Sleep(time.Second * 5) // sleep a bit to avoid race conditions where the pod is not quite up

	command := fmt.Sprintf("ls -ld %s", MOUNT_PATH)

	stdOut, stdErr, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, command, "")

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
	// fmt.Printf("Output: %s\n", stdOut) // uncomment this to get un-parsed command output

	if len(stdErr) > 0 {
		fmt.Printf("Error: %s\n", stdErr)
	}

	return (math.Abs(float64(strings.Compare(actualValue, expectedValue)))) == 0, actualValue, nil
}

func VerifyGroupIDIsUsed(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string,
	expectedValue string) (bool, string, error) {
	createFileCmd := fmt.Sprintf("touch %s/testfile.txt", MOUNT_PATH)
	testFileCmd := fmt.Sprintf("ls -l %s/testfile.txt", MOUNT_PATH)

	_, _, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, createFileCmd, "")

	if err != nil {
		fmt.Println("Error happened creating test file")
		return false, "", err
	}

	stdOut, stdErr, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, testFileCmd, "")

	if err != nil {
		fmt.Println("Error happened reading test file")
		return false, "", err
	}

	output := strings.Fields(stdOut)

	actualValue := strings.TrimSpace(output[3])

	fmt.Printf("Expected:%s\n", expectedValue)
	fmt.Printf("Actual: %s\n", actualValue)
	// fmt.Printf("Output: %s\n", stdOut) // uncomment if you want unparsed output

	if len(stdErr) > 0 {
		fmt.Printf("Error: %s\n", stdErr)
	}
	return (math.Abs(float64(strings.Compare(actualValue, expectedValue)))) == 0, actualValue, nil
}

func VerifyBlockWriteInPod(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string) (bool, string, error) {
	fmt.Printf("Testing for blockwrite in %s\n", podName)

	nodeNameCmd := "echo $KUBE_NODE_NAME"

	nodeName, stdErr, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, nodeNameCmd, "")

	if len(stdErr) > 0 {
		fmt.Printf("Error: %s\n", stdErr)
	}

	if err != nil {
		fmt.Printf("Error happened attempting to exec command in pod: %s\n", err.Error())
		return false, err.Error(), err
	}

	var charCount int // default to read no characters

	if len(nodeName) > 0 {
		charCount = len(strings.Fields(nodeName)[0])
	}

	// fmt.Printf("Nodename length is %d\n", charCount)

	testFileCmd := fmt.Sprintf("dd count=%d if=%s ibs=1 2>/dev/null", charCount, BLOCK_DEV_PATH)

	// fmt.Printf("Character count is: %d and testCmd: %s\n", charCount, testFileCmd)

	blockRead, stdErr2, err2 := execCmdInPod(ctx, clientSet, config, podName, nameSpace, testFileCmd, "")

	if len(stdErr2) > 0 {
		fmt.Printf("Error: %s\n", stdErr2)
	}

	if err2 != nil {
		fmt.Printf("Error happened attempting to exec command in pod: %s\n", err2.Error())
		return false, err2.Error(), err2
	}

	// fmt.Printf("Result from reading pod is: %s\n", blockRead)

	if strings.TrimSpace(nodeName) == strings.TrimSpace(blockRead) {
		return true, "", nil
	}
	return false, "Hostname did not match block written and read.", nil
}

// execCmdInPod - exec command on specific pod and wait the command's output.
func execCmdInPod(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string, command string, containerName string) (string, string, error) {
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
			Container: containerName,
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		},
		scheme.ParameterCodec,
	)

	// fmt.Printf("execCmdInPod - Running command: %s\n", command)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return stdOut.String(), stdErr.String(), err
	}
	err = exec.StreamWithContext(ctx,
		remotecommand.StreamOptions{
			Stdin:  nil,
			Stdout: stdOut,
			Stderr: stdErr,
		})

	return stdOut.String(), stdErr.String(), err
}

func VerifyReadOnlyMount(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string) error {
	catFileCmd := "cat /proc/mounts"

	stdOut, stdErr, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, catFileCmd, "")
	if err != nil {
		return err
	}

	if len(stdErr) > 0 {
		return fmt.Errorf("error: %s", stdErr)
	}

	// fmt.Printf("stdout %s\n", stdOut)

	mountLines := strings.Split(stdOut, "\n")

	for _, line := range mountLines {
		if strings.Contains(line, "csitesting") {
			lineTokens := strings.Split(line, " ")
			if len(lineTokens) < 4 {
				return fmt.Errorf("not enough tokens found in mount line for csitesting mount %d", len(lineTokens))
			}
			mountDetails := strings.Split(lineTokens[3], ",")
			for _, mountOption := range mountDetails {
				if mountOption == "ro" {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("could not find ro in the csitesting mount")
}

func CreateLinks(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string) error {
	// create a broken link
	createLinkCmd := "ln -s /tmp/monkey /tmp/csitesting/brokenlink"

	_, stdErr, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, createLinkCmd, "")
	if err != nil {
		return err
	}

	if len(stdErr) > 0 {
		return fmt.Errorf("error: %s", stdErr)
	}

	// fmt.Printf("cmd stdout %s\n", stdOut)

	// create a valid file
	validFileName := "/tmp/csitesting/validfile"
	createFileCmd := fmt.Sprintf("cp /etc/hosts %s", validFileName)

	_, stdErr, err = execCmdInPod(ctx, clientSet, config, podName, nameSpace, createFileCmd, "")
	if err != nil {
		return err
	}

	if len(stdErr) > 0 {
		return fmt.Errorf("error: %s", stdErr)
	}

	// fmt.Printf("cmd stdout %s\n", stdOut)

	// create a working sym link
	createValidLinkCmd := fmt.Sprintf("ln -s %s /tmp/csitesting/validlink", validFileName)

	_, stdErr, err = execCmdInPod(ctx, clientSet, config, podName, nameSpace, createValidLinkCmd, "")
	if err != nil {
		return err
	}

	if len(stdErr) > 0 {
		return fmt.Errorf("error: %s", stdErr)
	}

	// fmt.Printf("cmd stdout %s\n", stdOut)
	return nil
}

func GetMountSize(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string) (int64, error) {
	catFileCmd := "df -P /tmp/csitesting"

	stdOut, stdErr, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, catFileCmd, "")
	if err != nil {
		return 0, err
	}

	if len(stdErr) > 0 {
		return 0, fmt.Errorf("error: %s", stdErr)
	}

	// fmt.Printf("stdout %s\n", stdOut)

	mountLines := strings.Split(stdOut, "\n")

	if len(mountLines) < 3 {
		return 0, fmt.Errorf("df output is not correctly formatted %d lines", len(mountLines))
	}
	for _, line := range mountLines {
		fmt.Printf("line=[%s]\n", line)
	}

	var blocksString string
	contentLine := mountLines[1]
	fields := strings.Fields(contentLine)
	fmt.Printf("fields %+v\n", fields)
	if len(fields) < 2 {
		fmt.Printf("error in splitting df output into expected fields %+v\n", fields)
	}
	blocksString = fields[1]
	raw, err := strconv.Atoi(blocksString)
	if err != nil {
		fmt.Printf("error converting raw size into int %s\n", err.Error())
	}
	byteCount := raw * 1024
	fmt.Printf("1K blocks count %d - bytes size %d\n", raw, byteCount)

	return int64(byteCount), nil
}

func GetBlockVolumeSize(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string) (int64, error) {
	catFileCmd := "blockdev --getsize64 /dev/xvda"

	stdOut, stdErr, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, catFileCmd, "")
	if err != nil {
		return 0, err
	}

	if len(stdErr) > 0 {
		return 0, fmt.Errorf("error: %s", stdErr)
	}

	// fmt.Printf("stdout %s\n", stdOut)

	blockDeviceSizeString := strings.Fields(stdOut)

	if blockDeviceSizeString[0] == "" {
		return 0, fmt.Errorf("blockdev output is not correctly formatted %s lines", blockDeviceSizeString[0])
	}

	raw, err := strconv.Atoi(blockDeviceSizeString[0])
	if err != nil {
		fmt.Printf("error converting raw size into int %s\n", err.Error())
	}
	fmt.Printf("block device byte size %d \n", raw)

	return int64(raw), nil
}

func GetMpathDevicePath(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string) (string, error) {
	mountCmd := "mount | grep /tmp/csitesting"

	stdOut, stdErr, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, mountCmd, "")
	if err != nil {
		fmt.Printf("mount command stdErr %s \n", stdErr)
		return "", err
	}

	mountStrings := strings.Fields(stdOut)
	if len(mountStrings) == 0 {
		return "", fmt.Errorf("mount output is not correctly formatted len was zero %s", stdOut)
	}

	mpathDevicePath := mountStrings[0]
	if mpathDevicePath == "" {
		return "", fmt.Errorf("mount output is not correctly formatted %s lines", mpathDevicePath)
	}

	return mpathDevicePath, nil
}

func MpathExists(ctx context.Context, clientSet *kubernetes.Clientset, config *restclient.Config, podName string, nameSpace string, mpath string) (bool, error) {
	multipathCommand := "multipathd show multipaths 2> /dev/null | grep " + mpath + " | wc -l"

	stdOut, stdErr, err := execCmdInPod(ctx, clientSet, config, podName, nameSpace, multipathCommand, "driver")
	if err != nil {
		fmt.Printf("mount command stdErr %s \n", stdErr)
		return false, err
	}

	fmt.Printf("multipath command stdOut %s \n", stdOut)
	if stdOut == "" {
		return false, fmt.Errorf("multipath command output was not valid %s", stdOut)
	}

	count, err := strconv.Atoi(strings.TrimRight(stdOut, "\r\n"))
	if err != nil {
		return false, fmt.Errorf("multipath command output conversion error %s not valid %s", stdOut, err.Error())
	}
	if count > 0 {
		return true, nil
	}
	return false, nil
}

func CleanISCI(ctx context.Context, testConfig TestConfig) error {
	// find the csi driver node pod
	namespace := os.Getenv("_E2E_NAMESPACE")
	//fieldSelector := fmt.Sprintf("spec.nodeName=%s", testConfig.NodeName)
	labelSelector := "app=infinidat-csi-driver-node"
	listOptions := metav1.ListOptions{
		//FieldSelector: fieldSelector,
		LabelSelector: labelSelector,
	}
	csiPods, err := testConfig.ClientSet.CoreV1().Pods(namespace).List(ctx, listOptions)
	if err != nil {
		//return fmt.Errorf("error getting csi driver pod for nodeName %s fieldSelector %s labelSelector %s error %s", testConfig.NodeName, fieldSelector, labelSelector, err.Error())
		return fmt.Errorf("error getting csi driver pod for nodeName %s labelSelector %s error %s", testConfig.NodeName, labelSelector, err.Error())
	}

	for _, pod := range csiPods.Items {
		fmt.Printf("csi pod that matches is %s\n", pod.Name)
		iscsiLogoutCommand := "iscsiadm --mode node --logoutall=all"
		stdOut, stdErr, err := execCmdInPod(ctx, testConfig.ClientSet, testConfig.RestConfig, pod.Name, namespace, iscsiLogoutCommand, "driver")
		if err != nil {
			fmt.Printf("%s command stdOut %s stdErr %s\n", iscsiLogoutCommand, stdOut, stdErr)
			return err
		}

		// wait a bit to give iscsid a chance to work
		time.Sleep(time.Second * 5)
		iscsiNodeListCommand := "iscsiadm --mode node"
		stdOut, stdErr, err = execCmdInPod(ctx, testConfig.ClientSet, testConfig.RestConfig, pod.Name, namespace, iscsiNodeListCommand, "driver")
		if err != nil {
			fmt.Printf("%s command stdErr %s\n", iscsiNodeListCommand, stdErr)
			return err
		}

		nodeLines := strings.SplitSeq(stdOut, "\n")

		for line := range nodeLines {
			// fmt.Printf("line=[%s]\n", line)
			if line == "" {
				continue
			}

			ipAddressParts := strings.Split(line, ",")
			if len(ipAddressParts) != 2 {
				return fmt.Errorf("ip address parts did not parse correctly %d", len(ipAddressParts))
			}
			// fmt.Printf("ip [%s]\n", ipAddressParts[0])
			iqnParts := strings.Fields(ipAddressParts[1])
			if len(iqnParts) != 2 {
				return fmt.Errorf("iqn address parts did not parse correctly %d", len(iqnParts))
			}
			// fmt.Printf("iqn [%s]\n", iqnParts[1])
			cmdBase := fmt.Sprintf("iscsiadm -m node -o delete -T %s -p %s", iqnParts[1], ipAddressParts[0])
			fmt.Printf("%s\n", cmdBase)
			stdOut, stdErr, err = execCmdInPod(ctx, testConfig.ClientSet, testConfig.RestConfig, pod.Name, namespace, cmdBase, "driver")
			if err != nil {
				fmt.Printf("error cleaning ISCSI for pod %s on node %s -  %s command stdErr %s stdOut %s\n", pod.Name, testConfig.NodeName, cmdBase, stdErr, stdOut)
				return err
			}
		}
	}
	return nil
}

func FindDevicesForMpath(ctx context.Context, testConfig *TestConfig, mpath string) (devices []string, err error) {
	// find the csi driver node pod
	namespace := os.Getenv("_E2E_NAMESPACE")
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", testConfig.NodeName)
	labelSelector := "app=infinidat-csi-driver-node"
	listOptions := metav1.ListOptions{
		FieldSelector: fieldSelector,
		LabelSelector: labelSelector,
	}

	csiPods, err := testConfig.ClientSet.CoreV1().Pods(namespace).List(ctx, listOptions)
	if err != nil {
		return devices, fmt.Errorf("error getting csi driver pod for nodeName %s fieldSelector %s labelSelector %s error %s", testConfig.NodeName, fieldSelector, labelSelector, err.Error())
	}
	if len(csiPods.Items) != 1 {
		return devices, fmt.Errorf("too many driver node pods found %d", len(csiPods.Items))
	}

	nodePod := csiPods.Items[0]

	command := fmt.Sprintf("multipathd show multipath %s json", mpath)
	fmt.Printf("executing command %s\n", command)

	// we only care about the stdout, you can get stderro output from multipath.conf being misconfigured
	stdOut, stdErr, err := execCmdInPod(ctx, testConfig.ClientSet, testConfig.RestConfig, nodePod.Name, namespace, command, "driver")
	if err != nil {
		fmt.Printf("error looking up devices for mpath on pod %s on node %s -  %s command stdErr %s stdOut %s\n", nodePod.Name, testConfig.NodeName, command, stdErr, stdOut)
		return devices, err
	}

	var mpathOutput sc.ShowMultipathOutput
	err = json.Unmarshal([]byte(stdOut), &mpathOutput)
	if err != nil {
		e := fmt.Errorf("error unmarshalling output: %s, error: %s", stdOut, err)
		slog.Error(e.Error())
		return devices, e
	}

	pathGroups := mpathOutput.Map.PathGroups
	for i := range pathGroups {
		paths := pathGroups[i]
		for j := range paths.Paths {
			devices = append(devices, "/dev/"+paths.Paths[j].Dev)
		}
	}

	slog.Debug("list", "devices", devices, "for multipath", mpath)
	return devices, nil
}

func FileExists(ctx context.Context, testConfig *TestConfig, path string) (fileExists bool, err error) {
	// find the csi driver node pod
	namespace := os.Getenv("_E2E_NAMESPACE")
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", testConfig.NodeName)
	labelSelector := "app=infinidat-csi-driver-node"
	listOptions := metav1.ListOptions{
		FieldSelector: fieldSelector,
		LabelSelector: labelSelector,
	}

	csiPods, err := testConfig.ClientSet.CoreV1().Pods(namespace).List(ctx, listOptions)
	if err != nil {
		return fileExists, fmt.Errorf("error getting csi driver pod for nodeName %s fieldSelector %s labelSelector %s error %s", testConfig.NodeName, fieldSelector, labelSelector, err.Error())
	}
	if len(csiPods.Items) != 1 {
		return fileExists, fmt.Errorf("too many driver node pods found %d", len(csiPods.Items))
	}

	nodePod := csiPods.Items[0]

	command := fmt.Sprintf("ls /host%s", path)
	fmt.Printf("executing command %s\n", command)

	stdOut, stdErr, err := execCmdInPod(ctx, testConfig.ClientSet, testConfig.RestConfig, nodePod.Name, namespace, command, "driver")
	if err != nil {
		fmt.Printf("error looking up devices for mpath on pod %s on node %s -  %s command stdErr %s stdOut %s\n", nodePod.Name, testConfig.NodeName, command, stdErr, stdOut)
		return false, nil
	}
	fmt.Printf("command output %s\n", stdOut)
	if strings.Contains(stdOut, "No such file") {
		return false, nil
	}

	return true, nil
}

func GetMpathForBlockVolume(ctx context.Context, testConfig *TestConfig, pvcName string) (mpath string, err error) {
	getOptions := metav1.GetOptions{}
	pvc, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(ctx, pvcName, getOptions)
	if err != nil {
		return "", fmt.Errorf("error getting pvc %s error %s", pvcName, err.Error())
	}
	pv, err := testConfig.ClientSet.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, getOptions)
	if err != nil {
		return "", fmt.Errorf("error getting pv %s error %s", pvc.Spec.VolumeName, err.Error())
	}

	volumeHandle := pv.Spec.CSI.VolumeHandle
	volumeIDParts := strings.Split(volumeHandle, "$$")
	volumeID := volumeIDParts[0]
	fmt.Printf("volume ID: %s\n", volumeID)

	// find the csi driver node pod
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", testConfig.NodeName)
	labelSelector := "app=infinidat-csi-driver-node"
	listOptions := metav1.ListOptions{
		FieldSelector: fieldSelector,
		LabelSelector: labelSelector,
	}
	namespace := os.Getenv("_E2E_NAMESPACE")
	csiPods, err := testConfig.ClientSet.CoreV1().Pods(namespace).List(ctx, listOptions)
	if err != nil {
		return "", fmt.Errorf("error getting csi driver pod for nodeName %s fieldSelector %s labelSelector %s error %s", testConfig.NodeName, fieldSelector, labelSelector, err.Error())
	}
	if len(csiPods.Items) != 1 {
		return "", fmt.Errorf("too many driver node pods found %d", len(csiPods.Items))
	}

	nodePod := csiPods.Items[0]

	// look for a json config file in this location for that PV
	pvName := pvc.Spec.VolumeName
	volumeConfigPath := fmt.Sprintf("/var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/staging/%s/%s.json", pvName, volumeID)

	command := fmt.Sprintf("cat %s", volumeConfigPath)
	fmt.Printf("executing command %s\n", command)

	stdOut, stdErr, err := execCmdInPod(ctx, testConfig.ClientSet, testConfig.RestConfig, nodePod.Name, namespace, command, "driver")
	if err != nil {
		fmt.Printf("error looking up devices for mpath on pod %s on node %s -  %s command stdErr %s stdOut %s\n", nodePod.Name, testConfig.NodeName, command, stdErr, stdOut)
		return "", err
	}
	fmt.Printf("command output %s\n", stdOut)
	// we expect something like this:
	// {"rootdir":"/host","mpathdevice":"mpathc","isblock":false,"volumeid":1321676}

	var configFile sc.DiskInfo
	err = json.Unmarshal([]byte(stdOut), &configFile)
	if err != nil {
		log.Fatalf("Error parsing DiskInfo contents from JSON string: %v", err)
	}

	return configFile.MpathDevice, nil
}
