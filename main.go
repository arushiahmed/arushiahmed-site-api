package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/arushiahmed/arushiahmed-site-api/auth"
	"github.com/arushiahmed/arushiahmed-site-api/chat"
	"github.com/arushiahmed/arushiahmed-site-api/contact"
	"github.com/arushiahmed/arushiahmed-site-api/documents"
	"github.com/arushiahmed/arushiahmed-site-api/photos"
	"github.com/arushiahmed/arushiahmed-site-api/store"
	"github.com/arushiahmed/arushiahmed-site-api/uxdesigns"
)

func main() {
	photosBucket := envOrDefault("PHOTOS_BUCKET", "arushiahmed-photos")
	documentsBucket := envOrDefault("DOCUMENTS_BUCKET", "arushiahmed-documents")
	uxDesignsBucket := envOrDefault("UXDESIGNS_BUCKET", "arushiahmed-uxdesigns")
	// The uxdesigns bucket lives in a different region than the Lambda and
	// the other buckets; S3 GetObject (unlike ListObjectsV2) hard-fails with
	// a PermanentRedirect if the client's region doesn't match the bucket's,
	// so it needs its own correctly-configured client.
	uxDesignsBucketRegion := envOrDefault("UXDESIGNS_BUCKET_REGION", "us-east-1")

	photosCDNDomain := os.Getenv("PHOTOS_CDN_DOMAIN")
	documentsCDNDomain := os.Getenv("DOCUMENTS_CDN_DOMAIN")
	uxDesignsCDNDomain := os.Getenv("UXDESIGNS_CDN_DOMAIN")

	uxDesignsPasswordHash := mustEnv("UXDESIGNS_PASSWORD_HASH", "generate one with cmd/hashpassword")
	uxDesignsTokenSecret := mustEnv("UXDESIGNS_TOKEN_SECRET", "a long random string")

	bedrockRegion := envOrDefault("BEDROCK_REGION", "us-east-2")
	chatPromptKey := mustEnv("CHAT_PROMPT_KEY", "S3 key, in the documents bucket, of the chatbot's system prompt text file")

	sesRegion := envOrDefault("SES_REGION", "us-east-1")
	contactFromEmail := mustEnv("CONTACT_FROM_EMAIL", "a verified SES sender address")
	contactToEmail := mustEnv("CONTACT_TO_EMAIL", "the address that should receive contact form messages")

	cfg := mustLoadAWSConfig("default")
	uxDesignsCfg := mustLoadAWSConfig("uxdesigns", config.WithRegion(uxDesignsBucketRegion))
	bedrockCfg := mustLoadAWSConfig("bedrock", config.WithRegion(bedrockRegion))
	sesCfg := mustLoadAWSConfig("ses", config.WithRegion(sesRegion))

	s3Client := s3.NewFromConfig(cfg)
	photoSvc := photos.NewPhotoService(s3Client, photosBucket, photosCDNDomain)
	documentSvc := documents.NewDocumentService(s3Client, documentsBucket, documentsCDNDomain)
	uxDesignSvc := uxdesigns.NewUXDesignService(s3.NewFromConfig(uxDesignsCfg), uxDesignsBucket, uxDesignsCDNDomain, uxDesignsPasswordHash, []byte(uxDesignsTokenSecret))
	contactSvc := contact.NewContactService(sesv2.NewFromConfig(sesCfg), contactFromEmail, contactToEmail)

	chatPromptStore := store.New(s3Client, documentsBucket, "")
	chatSystemPrompt, err := chatPromptStore.GetObject(context.Background(), chatPromptKey)
	if err != nil {
		log.Fatalf("load chat system prompt from s3://%s/%s: %v", documentsBucket, chatPromptKey, err)
	}
	chatSvc := chat.NewChatService(bedrockruntime.NewFromConfig(bedrockCfg), string(chatSystemPrompt))

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
	mux.HandleFunc("POST /chat", chatSvc.Chat)
	mux.HandleFunc("POST /contact", contactSvc.Send)

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

// envOrDefault returns the named env var, or def if it's unset/empty.
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// mustEnv returns the named env var, or exits the process if it's unset.
// description explains what should go in it, for the resulting log message.
func mustEnv(key, description string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s must be set (%s)", key, description)
	}
	return v
}

// mustLoadAWSConfig loads an AWS config, or exits the process on failure.
// label identifies which config failed, for the resulting log message.
func mustLoadAWSConfig(label string, optFns ...func(*config.LoadOptions) error) aws.Config {
	cfg, err := config.LoadDefaultConfig(context.Background(), optFns...)
	if err != nil {
		log.Fatalf("load aws config for %s: %v", label, err)
	}
	return cfg
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func withCORS(next http.Handler) http.Handler {
	allowedOrigin := envOrDefault("ALLOWED_ORIGIN", "http://localhost:3000")
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
