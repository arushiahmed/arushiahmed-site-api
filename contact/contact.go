package contact

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// maxMessageLength bounds submission size (and therefore SES send cost/abuse
// potential) from an unauthenticated public endpoint.
const maxMessageLength = 5000

const maxSubjectLength = 200

// turnstileVerifyURL is Cloudflare Turnstile's server-side token verification
// endpoint. See https://developers.cloudflare.com/turnstile/get-started/server-side-validation/
const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type ContactService struct {
	client          *sesv2.Client
	fromAddress     string
	toAddress       string
	turnstileSecret string
	httpClient      *http.Client
}

func NewContactService(client *sesv2.Client, fromAddress, toAddress, turnstileSecret string) *ContactService {
	return &ContactService{
		client:          client,
		fromAddress:     fromAddress,
		toAddress:       toAddress,
		turnstileSecret: turnstileSecret,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

type contactRequest struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	Subject        string `json:"subject"`
	Message        string `json:"message"`
	TurnstileToken string `json:"turnstileToken"`
}

type turnstileVerifyResponse struct {
	Success bool `json:"success"`
}

// verifyTurnstile checks a Turnstile token server-side, per Cloudflare's
// documented siteverify contract. remoteIP is optional (best-effort signal,
// not required for verification to succeed).
func (s *ContactService) verifyTurnstile(token, remoteIP string) (bool, error) {
	form := url.Values{}
	form.Set("secret", s.turnstileSecret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	resp, err := s.httpClient.PostForm(turnstileVerifyURL, form)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result turnstileVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Success, nil
}

// clientIP returns the best-effort originating client IP for a request that
// may have arrived through CloudFront/Lambda Function URL proxying.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Send handles POST /contact: validates the submission and relays it as an
// email via SES. The visitor's address is set as Reply-To, so replying to
// the notification goes straight back to them rather than to fromAddress.
func (s *ContactService) Send(w http.ResponseWriter, r *http.Request) {
	var req contactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Subject = strings.TrimSpace(req.Subject)
	req.Message = strings.TrimSpace(req.Message)

	if req.Name == "" || req.Message == "" {
		http.Error(w, "name and message are required", http.StatusBadRequest)
		return
	}
	if len(req.Message) > maxMessageLength {
		http.Error(w, "message is too long", http.StatusBadRequest)
		return
	}
	if len(req.Subject) > maxSubjectLength {
		http.Error(w, "subject is too long", http.StatusBadRequest)
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		http.Error(w, "a valid email address is required", http.StatusBadRequest)
		return
	}
	if req.TurnstileToken == "" {
		http.Error(w, "verification token is required", http.StatusBadRequest)
		return
	}

	verified, err := s.verifyTurnstile(req.TurnstileToken, clientIP(r))
	if err != nil {
		log.Printf("verify turnstile token: %v", err)
		http.Error(w, "failed to verify request", http.StatusBadGateway)
		return
	}
	if !verified {
		http.Error(w, "verification failed", http.StatusForbidden)
		return
	}

	subject := "New message from " + req.Name + " via arushiahmed.com"
	if req.Subject != "" {
		subject = req.Subject + " — via arushiahmed.com contact form"
	}
	body := "From: " + req.Name + " <" + req.Email + ">\n\n" + req.Message

	_, err = s.client.SendEmail(r.Context(), &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.fromAddress),
		Destination:      &types.Destination{ToAddresses: []string{s.toAddress}},
		ReplyToAddresses: []string{req.Email},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String(subject)},
				Body:    &types.Body{Text: &types.Content{Data: aws.String(body)}},
			},
		},
	})
	if err != nil {
		log.Printf("send contact email: %v", err)
		http.Error(w, "failed to send message", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
