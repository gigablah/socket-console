package main

import (
	"github.com/codegangsta/negroni"
	"github.com/gorilla/websocket"
	"github.com/gorilla/mux"
	"github.com/kr/pty"

	"log"
	"net/http"
	"encoding/base64"
	"flag"
	"os"
	"os/exec"
)

var (
	cmdFlag string
	addr = ":9000"
	upgrader = websocket.Upgrader{
		ReadBufferSize: 1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

type wsPty struct {
	Cmd *exec.Cmd
	Pty *os.File
}

func (wp *wsPty) Start() {
	var err error
	args := flag.Args()
	wp.Cmd = exec.Command(cmdFlag, args...)
	wp.Pty, err = pty.Start(wp.Cmd)
	if err != nil {
		log.Fatalf("Failed to start command: %s\n", err)
	}
}

func (wp *wsPty) Stop() {
	wp.Pty.Close()
	err := wp.Cmd.Wait()
	if err != nil {
		log.Printf("Failed to complete command: %s\n", err)
	}
}

func consoleHandler(w http.ResponseWriter, r *http.Request) {
	c, ok := w.(http.CloseNotifier)
	if !ok {
		http.Error(w, "Close notification unsupported!\n", http.StatusInternalServerError)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Websocket upgrade failed: %s\n", err)
	}
	defer conn.Close()

	wp := wsPty{}
	wp.Start()
	defer wp.Stop()

	go func() {
		buf := make([]byte, 128)
		for {
			n, err := wp.Pty.Read(buf)
			if err != nil {
				log.Printf("Failed to read from pty master: %s\n", err)
				return
			}

			out := make([]byte, base64.StdEncoding.EncodedLen(n))
			base64.StdEncoding.Encode(out, buf[0:n])

			err = conn.WriteMessage(websocket.TextMessage, out)
			if err != nil {
				log.Printf("Failed to send %d bytes on websocket: %s\n", n, err)
				return
			}
		}
	}()

	closer := c.CloseNotify()

	for {
		select {
		case <-closer:
			log.Println("Closing connection\n")
			return
		}
	}
}

func init() {
	flag.StringVar(&cmdFlag, "cmd", "/usr/bin/top", "Command to execute")
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/console", consoleHandler)

	n := negroni.Classic()
	n.UseHandler(r)
	n.Run(addr)
}
