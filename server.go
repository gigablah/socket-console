package main

import (
	"github.com/codegangsta/negroni"
	"github.com/gorilla/websocket"
	"github.com/gorilla/mux"
	"github.com/nu7hatch/gouuid"

	"io"
	"fmt"
	"log"
	"net/http"
)

var (
	b *Broker
	addr = ":9000"
	upgrader = websocket.Upgrader{
		ReadBufferSize: 1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	proc = make(map[string]*Process)
)

func jobHandler(w http.ResponseWriter, r *http.Request) {
	u4, err := uuid.NewV4()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate UUID: %s\n", err), http.StatusInternalServerError)
		return
	}

	pid := u4.String()
	proc[pid] = NewProcess(pid, b.messages)

	go func() {
		//defer delete(proc, pid)
		proc[pid].Start()
	}()

	io.WriteString(w, fmt.Sprintf("Process %s created!\n", pid))
}

func consoleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pid := vars["pid"]

	_, ok := proc[pid]
	if !ok {
		http.Error(w, "Process not found!\n", http.StatusInternalServerError)
		return
	}

	c, ok := w.(http.CloseNotifier)
	if !ok {
		http.Error(w, "Close notification unsupported!\n", http.StatusInternalServerError)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if _, ok := err.(websocket.HandshakeError); ok {
			return
		}
		log.Printf("Websocket upgrade failed: %s\n", err)
	}
	defer ws.Close()

	// Create new channel for this client
	in := make(chan *Message)
	b.joining <- in
	defer func() {
		b.leaving <- in
	}()

	closer := c.CloseNotify()

	for {
		select {
		case m := <-in:
			if m.ID == pid {
				err := ws.WriteMessage(websocket.TextMessage, m.Body)
				if err != nil {
					log.Printf("Failed to write to websocket: %s\n", err)
					return
				}
			}
		case <-closer:
			log.Println("Closing connection\n")
			return
		}
	}
}

func main() {
	// Start the event broker
	b = NewBroker()
	b.Start()

	r := mux.NewRouter()
	r.HandleFunc("/console", jobHandler).Methods("POST")
	r.HandleFunc("/console/{pid}", consoleHandler)

	n := negroni.Classic()
	n.UseHandler(r)
	n.Run(addr)
}
