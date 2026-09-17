package uxdesigns

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/arushiahmed/arushiahmed-site-api/auth"
	"github.com/arushiahmed/arushiahmed-site-api/store"
)

// tokenTTL is how long a session issued by Auth stays valid.
const tokenTTL = 15 * time.Minute

type UXDesignService struct {
	store        *store.Service
	passwordHash string
	tokenSecret  []byte
}

func NewUXDesignService(client *s3.Client, bucket, cdnDomain, passwordHash string, tokenSecret []byte) *UXDesignService {
	return &UXDesignService{
		store:        store.New(client, bucket, cdnDomain),
		passwordHash: passwordHash,
		tokenSecret:  tokenSecret,
	}
}

// TokenSecret returns the secret used to sign and verify session tokens, for
// wrapping other handlers on this service with auth.Require.
func (s *UXDesignService) TokenSecret() []byte {
	return s.tokenSecret
}

type UXDesign = store.Item

type authRequest struct {
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
}

// Auth handles POST /uxdesigns/auth. On a correct password it returns a
// bearer token that authorizes the other /uxdesigns endpoints for tokenTTL.
func (s *UXDesignService) Auth(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Password == "" || !auth.CheckPassword(s.passwordHash, req.Password) {
		http.Error(w, "incorrect password", http.StatusUnauthorized)
		return
	}

	token := auth.IssueToken(s.tokenSecret, tokenTTL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(authResponse{Token: token})
}

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

// CaseStudy handles GET /uxdesigns/case-studies/{slug} and returns the
// contents of "{slug}/case-study.json" from the bucket directly (not a
// redirect — the frontend fetches this to render the protected case study).
func (s *UXDesignService) CaseStudy(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "missing project slug", http.StatusBadRequest)
		return
	}

	body, err := s.store.GetObject(r.Context(), slug+"/case-study.json")
	if err != nil {
		var notFound *s3types.NoSuchKey
		var respErr *smithyhttp.ResponseError
		if errors.As(err, &notFound) || (errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound) {
			http.Error(w, "case study not found", http.StatusNotFound)
			return
		}
		log.Printf("get case study %q: %v", slug, err)
		http.Error(w, "failed to load case study", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
