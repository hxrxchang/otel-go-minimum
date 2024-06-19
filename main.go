package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

type Message struct {
	Text string `json:"text"`
}

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(Message{Text: "Hello, World!"})
    })

	fmt.Println("Server is starting ...")

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigint
		fmt.Println("Server is stopped.")
		os.Exit(0)
	}()

    http.ListenAndServe(":8080", nil)
}
