package contact

import (
	"encoding/json"
	"log"
	"net/http"
	"net/mail"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// maxMessageLength bounds submission size (and therefore SES send cost/abuse
// potential) from an unauthenticated public endpoint.
const maxMessageLength = 5000

const maxSubjectLength = 200

type ContactService struct {
	client      *sesv2.Client
	fromAddress string
	toAddress   string
}

func NewContactService(client *sesv2.Client, fromAddress, toAddress string) *ContactService {
	return &ContactService{client: client, fromAddress: fromAddress, toAddress: toAddress}
}

type contactRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Subject string `json:"subject"`
	Message string `json:"message"`
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

	subject := "New message from " + req.Name + " via arushiahmed.com"
	if req.Subject != "" {
		subject = req.Subject + " — via arushiahmed.com contact form"
	}
	body := "From: " + req.Name + " <" + req.Email + ">\n\n" + req.Message

	_, err := s.client.SendEmail(r.Context(), &sesv2.SendEmailInput{
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
