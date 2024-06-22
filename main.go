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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)


func initTracer() (*sdktrace.TracerProvider, error) {
	client := otlptracehttp.NewClient(
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint("localhost:4318"),
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

func initStdOutTracer() (*sdktrace.TracerProvider, error) {
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
		log.Printf("Handling /cat")

		parentCtx, span := tracer.Start(r.Context(), "GET /cat")
		defer span.End()

		client := &http.Client{}

		childCtx, childSpan := tracer.Start(parentCtx, "HTTP GET: https://api.thecatapi.com/v1/images/search")

		req, err := http.NewRequestWithContext(parentCtx, "GET", "https://api.thecatapi.com/v1/images/search", nil)
		if err != nil {
			log.Printf("Error creating request: %v", err)
			span.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp, err := client.Do(req.WithContext(childCtx))
		if err != nil {
			log.Printf("Error making request: %v", err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		childSpan.End()

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading response body: %v", err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var data ApiResponse
		err = json.Unmarshal(body, &data)
		if err != nil {
			log.Printf("Error unmarshalling response: %v", err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{URL: data[0].URL})

		log.Printf("/cat handled successfully")
	})

	http.HandleFunc("/dog", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling /dog")

		ctx, span := tracer.Start(r.Context(), "GET /dog")
		defer span.End()

		client := &http.Client{}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.thedogapi.com/v1/images/search", nil)
		if err != nil {
			log.Printf("Error creating request: %v", err)
			span.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		childCtx, childSpan := tracer.Start(ctx, "HTTP GET: https://api.thedogapi.com/v1/images/search")

		resp, err := client.Do(req.WithContext(childCtx))
		if err != nil {
			log.Printf("Error making request: %v", err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		childSpan.End()
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading response body: %v", err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var data ApiResponse
		err = json.Unmarshal(body, &data)
		if err != nil {
			log.Printf("Error unmarshalling response: %v", err)
			childSpan.RecordError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{URL: data[0].URL})

		log.Printf("/dog handled successfully")
	})

	log.Println("Server is starting ...")

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigint
		log.Println("Server is stopped.")
		os.Exit(0)
	}()

	http.ListenAndServe(":8080", nil)
}

