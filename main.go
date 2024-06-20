package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

func initTracer() (*sdktrace.TracerProvider, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("cat-api-server"),
		)),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

type CatApiResponse []struct {
	URL string `json:"url"`
}

type Response struct {
	URL string `json:"url"`
}

func main() {
	tp, err := initTracer()
	if err != nil {
		fmt.Println("Error initializing tracer: ", err)
		os.Exit(1)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			fmt.Println("Error shutting down tracer: ", err)
		}
	}()

	tracer := otel.Tracer("cat-api-server")

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /")
		defer span.End()

		client := &http.Client{}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.thecatapi.com/v1/images/search", nil)
		if err != nil {
			span.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		startTime := time.Now()
		resp, err := client.Do(req)
		span.AddEvent("HTTP request made", trace.WithTimestamp(startTime))
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
