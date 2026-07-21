package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const presignExpiry = 15 * time.Minute

type PhotoService struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewPhotoService(client *s3.Client, bucket string) *PhotoService {
	return &PhotoService{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  bucket,
	}
}

type Photo struct {
	Key          string    `json:"key"`
	URL          string    `json:"url"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
}

// List handles GET /photos?prefix=optional
func (s *PhotoService) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefix := r.URL.Query().Get("prefix")

	photos := []Photo{}
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Printf("list objects: %v", err)
			http.Error(w, "failed to list photos", http.StatusBadGateway)
			return
		}
		for _, obj := range page.Contents {
			if obj.Key == nil || strings.HasSuffix(*obj.Key, "/") {
				continue
			}
			url, err := s.presignGet(ctx, *obj.Key)
			if err != nil {
				log.Printf("presign %s: %v", *obj.Key, err)
				continue
			}
			photos = append(photos, Photo{
				Key:          *obj.Key,
				URL:          url,
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photos)
}

// Get handles GET /photos/{key...} and redirects to a presigned S3 URL
func (s *PhotoService) Get(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "missing photo key", http.StatusBadRequest)
		return
	}

	url, err := s.presignGet(r.Context(), key)
	if err != nil {
		log.Printf("presign %s: %v", key, err)
		http.Error(w, "photo not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (s *PhotoService) presignGet(ctx context.Context, key string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(presignExpiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
