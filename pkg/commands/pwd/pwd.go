package pwd

import (
	"os"

	"github.com/xorrior/poseidon/pkg/utils/structs"
)

//Run - interface method that retrieves a process list
func Run(task structs.Task, threadChannel chan<- structs.ThreadMsg) {
	tMsg := structs.ThreadMsg{}
	tMsg.Error = false
	tMsg.TaskItem = task

	dir, err := os.Getwd()

	if err != nil {
		tMsg.Error = true
		tMsg.TaskResult = []byte(err.Error())
		threadChannel <- tMsg
		return
	}

	tMsg.TaskResult = []byte(dir)
	threadChannel <- tMsg
}
