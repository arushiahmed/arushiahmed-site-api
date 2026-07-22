package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
)

func main() {
	bucket := os.Getenv("PHOTOS_BUCKET")
	if bucket == "" {
		bucket = "arushiahmed-photos"
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}

	photos := NewPhotoService(s3.NewFromConfig(cfg), bucket)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("GET /photos", photos.List)
	mux.HandleFunc("GET /photos/{key...}", photos.Get)

	handler := withCORS(mux)

	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		// ALB path-pattern rules don't rewrite the path, so /api/* still
		// arrives with the /api prefix intact — strip it here so mux
		// route patterns stay identical between local dev and Lambda.
		adapter := httpadapter.NewALB(http.StripPrefix("/api", handler))
		lambda.Start(func(ctx context.Context, event events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
			resp, err := adapter.ProxyWithContext(ctx, event)
			// The adapter sets StatusDescription to just "OK" via
			// http.StatusText, but the ALB requires the "200 OK" format
			// (status code prefix) and returns 502 without it.
			resp.StatusDescription = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
			return resp, err
		})
		return
	}

	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func withCORS(next http.Handler) http.Handler {
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:3000"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
