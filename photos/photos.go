package photos

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/arushiahmed/arushiahmed-site-api/store"
)

type PhotoService struct {
	store *store.Service
}

func NewPhotoService(client *s3.Client, bucket string) *PhotoService {
	return &PhotoService{store: store.New(client, bucket)}
}

type Photo = store.Item

// List handles GET /photos?prefix=optional
func (s *PhotoService) List(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")

	photos, err := s.store.List(r.Context(), prefix, nil)
	if err != nil {
		log.Printf("list objects: %v", err)
		http.Error(w, "failed to list photos", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photos)
}

// ByCity handles GET /photos/city/{city} and returns every photo whose key
// contains the city name (case-insensitive), regardless of where in the key
// it appears.
func (s *PhotoService) ByCity(w http.ResponseWriter, r *http.Request) {
	city := r.PathValue("city")
	if city == "" {
		http.Error(w, "missing city", http.StatusBadRequest)
		return
	}
	needle := strings.ToLower(city)

	photos, err := s.store.List(r.Context(), "", func(key string) bool {
		return strings.Contains(strings.ToLower(key), needle)
	})
	if err != nil {
		log.Printf("list objects: %v", err)
		http.Error(w, "failed to list photos", http.StatusBadGateway)
		return
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

	url, err := s.store.PresignGet(r.Context(), key)
	if err != nil {
		log.Printf("presign %s: %v", key, err)
		http.Error(w, "photo not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}
