package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)


func initTracer() (*sdktrace.TracerProvider, error) {
	client := otlptracehttp.NewClient(
		otlptracehttp.WithInsecure(), // JaegerのOTLPエンドポイントがHTTPSでない場合に使用
		otlptracehttp.WithEndpoint("localhost:4318"), // OTLPエンドポイントを指定
	)

	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("cat-api-server"),
		)),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

type ApiResponse []struct {
	URL string `json:"url"`
}

type Response struct {
	URL string `json:"url"`
}

func main() {
	tp, err := initTracer()
	if err != nil {
		fmt.Printf("failed to initialize tracer: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			fmt.Printf("failed to shutdown tracer: %v\n", err)
		}
	}()

	tracer := otel.Tracer("cat-api-server")

	http.HandleFunc("/cat", func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()
		log.Printf("Handling request ID: %s", requestID)

		ctx, span := tracer.Start(r.Context(), "GET /cat")
		defer span.End()

		client := &http.Client{}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.thecatapi.com/v1/images/search", nil)
		if err != nil {
			log.Printf("Request ID: %s, Error creating request: %v", requestID, err)
			span.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		childCtx, childSpan := tracer.Start(ctx, "HTTP GET: https://api.thecatapi.com/v1/images/search")

		resp, err := client.Do(req.WithContext(childCtx))
		if err != nil {
			log.Printf("Request ID: %s, Error making request: %v", requestID, err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		childSpan.End()

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Request ID: %s, Error reading response body: %v", requestID, err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var data ApiResponse
		err = json.Unmarshal(body, &data)
		if err != nil {
			log.Printf("Request ID: %s, Error unmarshalling response: %v", requestID, err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{URL: data[0].URL})

		log.Printf("Request ID: %s, Request handled successfully", requestID)
	})

	http.HandleFunc("/dog", func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()
		log.Printf("Handling request ID: %s", requestID)

		ctx, span := tracer.Start(r.Context(), "GET /dog")
		defer span.End()

		client := &http.Client{}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.thedogapi.com/v1/images/search", nil)
		if err != nil {
			log.Printf("Request ID: %s, Error creating request: %v", requestID, err)
			span.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		childCtx, childSpan := tracer.Start(ctx, "HTTP GET: https://api.thedogapi.com/v1/images/search")

		resp, err := client.Do(req.WithContext(childCtx))
		if err != nil {
			log.Printf("Request ID: %s, Error making request: %v", requestID, err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		childSpan.End()
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Request ID: %s, Error reading response body: %v", requestID, err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var data ApiResponse
		err = json.Unmarshal(body, &data)
		if err != nil {
			log.Printf("Request ID: %s, Error unmarshalling response: %v", requestID, err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{URL: data[0].URL})

		log.Printf("Request ID: %s, Request handled successfully", requestID)
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

