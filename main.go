package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

type CatApiResponse []struct {
	URL string `json:"url"`
}

type Response struct {
	URL string `json:"url"`
}

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Get("https://api.thecatapi.com/v1/images/search")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		var data CatApiResponse
		err = json.Unmarshal(body, &data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(Response{URL: data[0].URL})
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
