/*
Copyright 2026 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	go catchSignal()

	fmt.Printf("starting csitest block ... current user id %d group id %d\n", os.Getuid(), os.Getgid())
	disk := "/dev/xvda"
	valueToWrite := "foo"

	readOnlyEnvVar := os.Getenv("READ_ONLY")
	var readOnly bool
	var err error
	if readOnlyEnvVar != "" {
		readOnly, err = strconv.ParseBool(readOnlyEnvVar)
		if err != nil {
			fmt.Printf("error parsing READ_ONLY env var %s", err.Error())
			os.Exit(2)
		}
	}
	if readOnly {
		fmt.Println("READ_ONLY is true")
	}

	// use nodeName if it exists
	nodeName := os.Getenv("KUBE_NODE_NAME")
	if nodeName != "" {
		valueToWrite = nodeName
	}

	listPermissions(disk)

	if readOnly {
		n, valueRead, err := readFromBlockDevice(valueToWrite, disk)
		if err != nil {
			fmt.Printf("error reading %s\n", err.Error())
			os.Exit(1)
		}
		fmt.Printf("read %d bytes [%s] from block device %s\n", n, valueRead, disk)
	} else {
		err := writeToBlockDevice(valueToWrite, disk)
		if err != nil {
			fmt.Printf("error writing %s\n", err.Error())
			os.Exit(1)
		}
	}

	for {
		time.Sleep(time.Second * 30)
		fmt.Println(".")
	}
}

func catchSignal() {
	terminateSignals := make(chan os.Signal, 1)

	signal.Notify(terminateSignals, syscall.SIGINT, syscall.SIGTERM) //NOTE:: syscall.SIGKILL we cannot catch kill -9 as its force kill signal.

	for s := range terminateSignals {
		fmt.Printf("Got one of stop signals, shutting down gracefully, SIGNAL NAME : %v\n", s)
		os.Exit(1)
	}
}

func writeToBlockDevice(value, device string) error {
	// simulate the command line: echo foo > /dev/xvda;sync
	file, err := syscall.Open(device, os.O_RDWR, 0777)
	if err != nil {
		fmt.Printf("error in opening block device %s\n", err.Error())
		return err
	}

	defer func() {
		if err := syscall.Close(file); err != nil {
			panic(err)
		}
	}()

	// write a chunk
	buffer := []byte(value)
	if _, err := syscall.Write(file, buffer); err != nil {
		panic(err)
	}

	syscall.Sync()

	return nil
}

func readFromBlockDevice(value, device string) (int, string, error) {
	// simulate the command line: head -c 3 /dev/xvda
	file, err := syscall.Open(device, os.O_RDONLY, 0555)
	if err != nil {
		fmt.Printf("error in opening block device %s\n", err.Error())
		return 0, "", err
	}

	defer func() {
		if err := syscall.Close(file); err != nil {
			panic(err)
		}
	}()

	buffer := make([]byte, len(value))
	bytesRead, err := syscall.Read(file, buffer)
	if err != nil && err != io.EOF {
		panic(err)
	}

	valueRead := string(buffer[:bytesRead])

	return bytesRead, valueRead, nil
}

func listPermissions(device string) {
	cmd := exec.Command("ls", "-l", device)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("error in ls block device %s\n", err.Error())
	}
	fmt.Printf("ls -l %s\n", device)
	fmt.Printf("%s\n", output)
}
