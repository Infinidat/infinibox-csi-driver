package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {

	fmt.Printf("current user id %d\n", os.Getuid())
	fmt.Printf("current group id %d\n", os.Getgid())

	fi, err := openFile()
	if err != nil {
		fmt.Printf("error opening file %s", err.Error())
		os.Exit(2)
	}

	go catchSignal()

	fmt.Println("starting csitest...")

	for {
		writeToFile(fi)
		time.Sleep(time.Second * 10)
	}

}

func catchSignal() {

	terminateSignals := make(chan os.Signal, 1)

	signal.Notify(terminateSignals, syscall.SIGINT, syscall.SIGTERM) //NOTE:: syscall.SIGKILL we cannot catch kill -9 as its force kill signal.

	for s := range terminateSignals {
		log.Println("Got one of stop signals, shutting down gracefully, SIGNAL NAME :", s)
		os.Exit(1)
		break
	}

}

func openFile() (*os.File, error) {
	var fi *os.File
	var fileName = "/tmp/csitesting/testfile"

	_, err := os.Stat(fileName)
	if err != nil {
		fmt.Println("file does not exist, will create...")
		fi, err = os.Create(fileName)
		if err != nil {
			fmt.Printf("error creating file %s\n", err.Error())
			return nil, err
		}
	} else {
		fmt.Println("file already exists...")
		fi, err = os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
		if err != nil {
			fmt.Printf("error opening file %s\n", err.Error())
			return nil, err
		}
	}
	return fi, nil
}

func writeToFile(fi *os.File) {
	_, err := fi.WriteString("w")
	if err != nil {
		fmt.Printf("error writing to file %s\n", err.Error())
		os.Exit(2)
	}

}
