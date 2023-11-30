package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {

	go catchSignal()

	fmt.Printf("current user id %d\n", os.Getuid())
	fmt.Printf("current group id %d\n", os.Getgid())

	fmt.Println("starting csitest block...")

	disk := "/dev/xvda"
	valueToWrite := "foo"

	listPermissions(disk)

	err := writeToBlockDevice(valueToWrite, disk)
	if err != nil {
		os.Exit(1)
	}

	time.Sleep(time.Second * 4)

	err = readFromBlockDevice(valueToWrite, disk)
	if err != nil {
		fmt.Printf("error reading %s\n", err.Error())
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
	f, err := syscall.Open(device, os.O_RDWR, 0777)
	if err != nil {
		fmt.Printf("error in opening block device %s\n", err.Error())
		return err
	}

	defer func() {
		if err := syscall.Close(f); err != nil {
			panic(err)
		}
	}()

	// write a chunk
	buf := []byte(value)
	if _, err := syscall.Write(f, buf); err != nil {
		panic(err)
	}

	syscall.Sync()

	return nil
}

func readFromBlockDevice(value, device string) error {

	// simulate the command line: head -c 3 /dev/xvda

	f, err := syscall.Open(device, os.O_RDWR, 0777)
	if err != nil {
		fmt.Printf("error in opening block device %s\n", err.Error())
		return err
	}

	defer func() {
		if err := syscall.Close(f); err != nil {
			panic(err)
		}
	}()

	buf := make([]byte, len(value))
	n, err := syscall.Read(f, buf)
	if err != nil && err != io.EOF {
		panic(err)
	}

	valueRead := string(buf[:n])
	fmt.Printf("read %d bytes [%s] from block device %s\n", n, valueRead, device)
	if valueRead != value {
		return fmt.Errorf("value of [%s] len %d did not match the expected value of [%s] len %d", valueRead, len(valueRead), value, len(value))
	}

	return nil
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
