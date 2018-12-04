package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func Exec(command []string) (string, string, int) {

	var timeOut time.Duration
	timeOut = 30

	c, command := command[0], command[1:]

	cmd := exec.Command(c, command...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start command asynchronously
	err := cmd.Start()
	if err != nil {
		os.Stderr.WriteString(err.Error())
	}

	timer := time.NewTimer(time.Second * timeOut)
	go func(timer *time.Timer, cmd *exec.Cmd) {
		for _ = range timer.C {
			err := cmd.Process.Signal(os.Kill)
			if err != nil {
				os.Stderr.WriteString(err.Error())
			}
		}
	}(timer, cmd)

	// Wait for the command to finish
	cmd.Wait()
	if exiterr, ok := err.(*exec.ExitError); ok {
		// The program has exited with an exit code != 0

		status, ok := exiterr.Sys().(syscall.WaitStatus)
		if ok {
			log.Printf("Exit Status: %d\n", status.ExitStatus())
			return stdout.String(), stderr.String(), status.ExitStatus()
		} else {
			return stdout.String(), stderr.String(), 0
		}
	} else {
		//log.Fatalf("cmd.Wait: %v", err)
	}
	return stdout.String(), stderr.String(), 0
}

//
