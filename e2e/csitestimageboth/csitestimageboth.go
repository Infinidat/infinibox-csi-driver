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
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var oneWrite, oneRead bool

func main() {
	fmt.Printf("current user id %d\n", os.Getuid())
	fmt.Printf("current group id %d\n", os.Getgid())

	readOnlyEnvVar := os.Getenv("READ_ONLY")
	var readOnly bool
	var err error

	// block device
	disk := "/dev/xvda"
	valueToWrite := "foo"

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

	file, err := openFile(readOnly)
	if err != nil {
		fmt.Printf("error opening file %s", err.Error())
		os.Exit(2)
	}

	go catchSignal()

	fmt.Println("starting csitest...")

	for {
		if readOnly && !oneRead {
			readFile(file)
			n, valueRead, err := readFromBlockDevice(valueToWrite, disk)
			if err != nil {
				fmt.Printf("error reading %s\n", err.Error())
				os.Exit(1)
			}
			fmt.Printf("read %d bytes [%s] from block device %s\n", n, valueRead, disk)
			oneRead = true
		} else {
			if !oneWrite {
				writeToFile(file)
				err := writeToBlockDevice(valueToWrite, disk)
				if err != nil {
					fmt.Printf("error writing %s\n", err.Error())
				}
				oneWrite = true
			}
		}
		time.Sleep(time.Second * 30)
	}
}

func catchSignal() {
	terminateSignals := make(chan os.Signal, 1)

	signal.Notify(terminateSignals, syscall.SIGINT, syscall.SIGTERM) //NOTE:: syscall.SIGKILL we cannot catch kill -9 as its force kill signal.

	for signal := range terminateSignals {
		log.Println("Got one of stop signals, shutting down gracefully, SIGNAL NAME :", signal)
		os.Exit(1)
		break
	}
}

func openFile(readOnly bool) (*os.File, error) {
	var file *os.File
	var fileName = "/tmp/csitesting/testfile"

	_, err := os.Stat(fileName)
	if err != nil {
		if !readOnly {
			fmt.Println("file does not exist, will create...")
			file, err = os.Create(fileName)
			if err != nil {
				fmt.Printf("error creating file %s\n", err.Error())
				return nil, err
			}
		}
	} else {
		fmt.Println("file already exists...")
		if readOnly {
			file, err = os.Open(fileName)
			fmt.Println("opening in read-only mode")
		} else {
			file, err = os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
		}
		if err != nil {
			fmt.Printf("error opening file %s\n", err.Error())
			return nil, err
		}
	}
	return file, nil
}

func writeToFile(file *os.File) {
	_, err := file.WriteString("w")
	if err != nil {
		fmt.Printf("error writing to file %s\n", err.Error())
		os.Exit(2)
	}
}

func readFile(file *os.File) {
	var totalBytes int
	buffer := make([]byte, 1024)
	for {
		bytesRead, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			// fmt.Println(err)
			continue
		}
		if bytesRead > 0 {
			// fmt.Println(string(buf[:n]))
			totalBytes += bytesRead
		}
	}
	fmt.Printf("%d read from file content [%s]\n", totalBytes, string(buffer[:totalBytes]))
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
