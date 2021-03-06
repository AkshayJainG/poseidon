// +build windows

package shell

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/google/shlex"
)

//WindowsShell - struct to hold the task and result of the shell command
type WindowsShell struct {
	Task       string
	TaskResult []byte
}

//Command - interface method that returns the command
func (d *WindowsShell) Command() string {
	return d.Task
}

//Response - interface method that holds the response to the command
func (d *WindowsShell) Response() []byte {
	return d.TaskResult
}

func shellExec(c string) (Shell, error) {
	c = fmt.Sprintf("cmd.exe /c %s", c)
	args, _ := shlex.Split(c)
	if len(args) < 3 {
		return nil, errors.New("Not enough arguments given for shell command.")
	}

	cmd := exec.Command(args[0], args[1:]...)

	r := &WindowsShell{}

	out, err := cmd.CombinedOutput()

	if len(out) == 0 && err != nil {
		return nil, err
	} else if len(out) == 0 && err == nil {
		r.Task = c
		r.TaskResult = []byte("Task completed with exit code 0.")
	} else {
		r.Task = c
		r.TaskResult = out
	}

	return r, nil
}
