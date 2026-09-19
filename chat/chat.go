package chat

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// maxHistoryMessages caps how many prior turns a client can send, bounding
// the token cost (and therefore price) of any single request.
const maxHistoryMessages = 20

// modelID is Claude Haiku 4.5's Bedrock model ID.
const modelID = "anthropic.claude-haiku-4-5"

type ChatService struct {
	client       *bedrockruntime.Client
	systemPrompt string
}

func NewChatService(client *bedrockruntime.Client, systemPrompt string) *ChatService {
	return &ChatService{client: client, systemPrompt: systemPrompt}
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

	messages := make([]types.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		content := []types.ContentBlock{&types.ContentBlockMemberText{Value: m.Content}}
		switch m.Role {
		case "user":
			messages = append(messages, types.Message{Role: types.ConversationRoleUser, Content: content})
		case "assistant":
			messages = append(messages, types.Message{Role: types.ConversationRoleAssistant, Content: content})
		default:
			http.Error(w, "each message role must be \"user\" or \"assistant\"", http.StatusBadRequest)
			return
		}
	}

	resp, err := s.client.Converse(r.Context(), &bedrockruntime.ConverseInput{
		ModelId:  aws.String(modelID),
		Messages: messages,
		System:   []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: s.systemPrompt}},
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens: aws.Int32(1024),
		},
	})
	if err != nil {
		log.Printf("chat completion: %v", err)
		http.Error(w, "failed to get a response", http.StatusBadGateway)
		return
	}

	var reply strings.Builder
	if out, ok := resp.Output.(*types.ConverseOutputMemberMessage); ok {
		for _, block := range out.Value.Content {
			if text, ok := block.(*types.ContentBlockMemberText); ok {
				reply.WriteString(text.Value)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResponse{Reply: reply.String()})
}
