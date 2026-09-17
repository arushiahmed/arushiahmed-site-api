package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/arushiahmed/arushiahmed-site-api/auth"
	"github.com/arushiahmed/arushiahmed-site-api/documents"
	"github.com/arushiahmed/arushiahmed-site-api/photos"
	"github.com/arushiahmed/arushiahmed-site-api/uxdesigns"
)

func main() {
	photosBucket := os.Getenv("PHOTOS_BUCKET")
	if photosBucket == "" {
		photosBucket = "arushiahmed-photos"
	}

	documentsBucket := os.Getenv("DOCUMENTS_BUCKET")
	if documentsBucket == "" {
		documentsBucket = "arushiahmed-documents"
	}

	uxDesignsBucket := os.Getenv("UXDESIGNS_BUCKET")
	if uxDesignsBucket == "" {
		uxDesignsBucket = "arushiahmed-uxdesigns"
	}
	// The uxdesigns bucket lives in a different region than the Lambda and
	// the other buckets; S3 GetObject (unlike ListObjectsV2) hard-fails with
	// a PermanentRedirect if the client's region doesn't match the bucket's,
	// so it needs its own correctly-configured client.
	uxDesignsBucketRegion := os.Getenv("UXDESIGNS_BUCKET_REGION")
	if uxDesignsBucketRegion == "" {
		uxDesignsBucketRegion = "us-east-1"
	}

	photosCDNDomain := os.Getenv("PHOTOS_CDN_DOMAIN")
	documentsCDNDomain := os.Getenv("DOCUMENTS_CDN_DOMAIN")
	uxDesignsCDNDomain := os.Getenv("UXDESIGNS_CDN_DOMAIN")

	uxDesignsPasswordHash := os.Getenv("UXDESIGNS_PASSWORD_HASH")
	if uxDesignsPasswordHash == "" {
		log.Fatal("UXDESIGNS_PASSWORD_HASH must be set (generate one with cmd/hashpassword)")
	}
	uxDesignsTokenSecret := os.Getenv("UXDESIGNS_TOKEN_SECRET")
	if uxDesignsTokenSecret == "" {
		log.Fatal("UXDESIGNS_TOKEN_SECRET must be set to a long random string")
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	uxDesignsCfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(uxDesignsBucketRegion))
	if err != nil {
		log.Fatalf("load aws config for uxdesigns: %v", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	photoSvc := photos.NewPhotoService(s3Client, photosBucket, photosCDNDomain)
	documentSvc := documents.NewDocumentService(s3Client, documentsBucket, documentsCDNDomain)
	uxDesignSvc := uxdesigns.NewUXDesignService(s3.NewFromConfig(uxDesignsCfg), uxDesignsBucket, uxDesignsCDNDomain, uxDesignsPasswordHash, []byte(uxDesignsTokenSecret))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("GET /photos", photoSvc.List)
	mux.HandleFunc("GET /photos/city/{city}", photoSvc.ByCity)
	mux.HandleFunc("GET /photos/{key...}", photoSvc.Get)
	mux.HandleFunc("GET /documents", documentSvc.List)
	mux.HandleFunc("GET /documents/{key...}", documentSvc.Get)
	mux.HandleFunc("POST /uxdesigns/auth", uxDesignSvc.Auth)
	mux.HandleFunc("GET /uxdesigns", auth.Require(uxDesignSvc.TokenSecret(), uxDesignSvc.List))
	mux.HandleFunc("GET /uxdesigns/case-studies/{slug}", auth.Require(uxDesignSvc.TokenSecret(), uxDesignSvc.CaseStudy))
	mux.HandleFunc("GET /uxdesigns/{key...}", auth.Require(uxDesignSvc.TokenSecret(), uxDesignSvc.Get))

	handler := withCORS(mux)

	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		// CloudFront's /api/* path pattern doesn't rewrite the path, so it
		// still arrives with the /api prefix intact — strip it here so mux
		// route patterns stay identical between local dev and Lambda.
		adapter := httpadapter.NewV2(http.StripPrefix("/api", handler))
		lambda.Start(adapter.ProxyWithContext)
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
