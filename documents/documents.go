package documents

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

type DocumentService struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewDocumentService(client *s3.Client, bucket string) *DocumentService {
	return &DocumentService{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  bucket,
	}
}

type Document struct {
	Key          string    `json:"key"`
	URL          string    `json:"url"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
}

// List handles GET /documents?prefix=optional
func (s *DocumentService) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefix := r.URL.Query().Get("prefix")

	documents := []Document{}
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Printf("list objects: %v", err)
			http.Error(w, "failed to list documents", http.StatusBadGateway)
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
			documents = append(documents, Document{
				Key:          *obj.Key,
				URL:          url,
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(documents)
}

// Get handles GET /documents/{key...} and redirects to a presigned S3 URL
func (s *DocumentService) Get(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "missing document key", http.StatusBadRequest)
		return
	}

	url, err := s.presignGet(r.Context(), key)
	if err != nil {
		log.Printf("presign %s: %v", key, err)
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (s *DocumentService) presignGet(ctx context.Context, key string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(presignExpiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
