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
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
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

type CatApiResponse []struct {
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

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		requestID := uuid.New().String()
		log.Printf("Handling request ID: %s", requestID)

		ctx, span := tracer.Start(r.Context(), "handleRequest")
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
		defer childSpan.End()

		startTime := time.Now()
		childSpan.AddEvent("HTTP request made", trace.WithTimestamp(startTime))

		resp, err := client.Do(req.WithContext(childCtx))
		if err != nil {
			log.Printf("Request ID: %s, Error making request: %v", requestID, err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Request ID: %s, Error reading response body: %v", requestID, err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var data CatApiResponse
		err = json.Unmarshal(body, &data)
		if err != nil {
			log.Printf("Request ID: %s, Error unmarshalling response: %v", requestID, err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		endTime := time.Now()
		duration := endTime.Sub(startTime)
		childSpan.AddEvent("HTTP request completed", trace.WithAttributes(
			attribute.String("response.time", duration.String()),
		))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{URL: data[0].URL})

		log.Printf("Request ID: %s, Request handled successfully in %v", requestID, duration)
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

