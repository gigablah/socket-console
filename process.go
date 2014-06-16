package main

import (
	"github.com/kr/pty"

	"log"
	"flag"
	"os"
	"os/exec"
	"syscall"
	"encoding/base64"
)

var cmdFlag string

func init() {
	flag.StringVar(&cmdFlag, "cmd", "/usr/bin/htop", "Command to execute")
}

// A Process executes a command through a pseudo-terminal
// and sends the output to a channel.
type Process struct {
	id   string
	out  chan<- *Message
	done chan struct{}
	cmd  *exec.Cmd
	pty  *os.File
}

// Start the process and send output.
func (p *Process) Start() {
	var err error
	args := flag.Args()
	p.cmd = exec.Command(cmdFlag, args...)
	p.pty, err = pty.Start(p.cmd)
	if err != nil {
		log.Fatalf("Failed to start command: %s\n", err)
		// pty has called Close()
		return
	}
	defer p.pty.Close()

	go p.Wait()

	buf := make([]byte, 128)
	for {
		select {
    	case <-p.done:
    		return
    	default:
	 		n, err := p.pty.Read(buf)
			if err != nil {
				log.Printf("Failed to read from pty master: %s\n", err)
				<-p.done
				return
			}

			msg := make([]byte, base64.StdEncoding.EncodedLen(n))
			base64.StdEncoding.Encode(msg, buf[0:n])

			p.out <- &Message{ID: p.id, Type: "msg", Body: msg}
    	}
	}
}

// Kill a running process.
func (p *Process) Kill() {
	if p == nil {
        return
    }
    p.cmd.Process.Kill()
    <-p.done
}

// Wait for a process to exit and inform any clients.
func (p *Process) Wait() {
	m := &Message{ID: p.id, Type: "end"}
	if err := p.cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.ProcessState.Sys().(syscall.WaitStatus); ok {
				log.Printf("Exit status: %d\n", status.ExitStatus())
			}
		}
		log.Printf("Failed to complete command: %s\n", err)
	}
	p.out <- m
	close(p.done)
}

// NewProcess creates a new Process.
func NewProcess(id string, out chan<- *Message) *Process {
    return &Process{
    	id: id,
    	out: out,
    	done: make(chan struct{}),
    }
}
