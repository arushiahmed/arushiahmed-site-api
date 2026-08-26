package uxdesigns

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/arushiahmed/arushiahmed-site-api/store"
)

type UXDesignService struct {
	store *store.Service
}

func NewUXDesignService(client *s3.Client, bucket, cdnDomain string) *UXDesignService {
	return &UXDesignService{store: store.New(client, bucket, cdnDomain)}
}

type UXDesign = store.Item

// List handles GET /uxdesigns?prefix=optional
func (s *UXDesignService) List(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")

	designs, err := s.store.List(r.Context(), prefix, nil)
	if err != nil {
		log.Printf("list objects: %v", err)
		http.Error(w, "failed to list ux designs", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(designs)
}

// Get handles GET /uxdesigns/{key...} and redirects to the design's CDN URL
func (s *UXDesignService) Get(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "missing design key", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, s.store.PublicURL(key), http.StatusFound)
}
