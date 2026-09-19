package chat

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
)

// maxHistoryMessages caps how many prior turns a client can send, bounding
// the token cost (and therefore price) of any single request.
const maxHistoryMessages = 20

// model is Claude Haiku 4.5's Bedrock model ID (Bedrock model IDs carry an
// "anthropic." prefix that the direct Anthropic API doesn't use).
const model anthropic.Model = "anthropic.claude-haiku-4-5"

type ChatService struct {
	client       *bedrock.MantleClient
	systemPrompt string
}

// NewChatService authenticates via the Lambda's own AWS credentials (the
// same default credential chain the S3 clients use) rather than a separate
// Anthropic API key.
func NewChatService(ctx context.Context, awsRegion, systemPrompt string) (*ChatService, error) {
	client, err := bedrock.NewMantleClient(ctx, bedrock.MantleClientConfig{AWSRegion: awsRegion})
	if err != nil {
		return nil, err
	}
	return &ChatService{client: client, systemPrompt: systemPrompt}, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Reply string `json:"reply"`
}

// Chat handles POST /chat. The client sends the full conversation history
// (this API is stateless, like the rest of the app) and gets back Claude's
// next reply.
func (s *ChatService) Chat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "messages must not be empty", http.StatusBadRequest)
		return
	}
	if len(req.Messages) > maxHistoryMessages {
		req.Messages = req.Messages[len(req.Messages)-maxHistoryMessages:]
	}

	messages := make([]anthropic.MessageParam, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		default:
			http.Error(w, "each message role must be \"user\" or \"assistant\"", http.StatusBadRequest)
			return
		}
	}

	resp, err := s.client.Messages.New(r.Context(), anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: s.systemPrompt}},
		Messages:  messages,
	})
	if err != nil {
		log.Printf("chat completion: %v", err)
		http.Error(w, "failed to get a response", http.StatusBadGateway)
		return
	}

	var reply strings.Builder
	for _, block := range resp.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			reply.WriteString(text.Text)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResponse{Reply: reply.String()})
}
