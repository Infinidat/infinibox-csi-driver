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

func main() {
	fmt.Printf("current user id %d\n", os.Getuid())
	fmt.Printf("current group id %d\n", os.Getgid())

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

	file, err := openFile(readOnly)
	if err != nil {
		fmt.Printf("error opening file %s", err.Error())
		os.Exit(2)
	}

	go catchSignal()

	fmt.Println("starting csitest...")

	for {
		if readOnly {
			readFile(file)
		} else {
			writeToFile(file)
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
	fmt.Printf("%d read\n", totalBytes)
}
