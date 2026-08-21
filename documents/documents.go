package documents

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/arushiahmed/arushiahmed-site-api/store"
)

type DocumentService struct {
	store *store.Service
}

func NewDocumentService(client *s3.Client, bucket, cdnDomain string) *DocumentService {
	return &DocumentService{store: store.New(client, bucket, cdnDomain)}
}

type Document = store.Item

// List handles GET /documents?prefix=optional
func (s *DocumentService) List(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")

	documents, err := s.store.List(r.Context(), prefix, nil)
	if err != nil {
		log.Printf("list objects: %v", err)
		http.Error(w, "failed to list documents", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(documents)
}

// Get handles GET /documents/{key...} and redirects to the document's CDN URL
func (s *DocumentService) Get(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "missing document key", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, s.store.PublicURL(key), http.StatusFound)
}
