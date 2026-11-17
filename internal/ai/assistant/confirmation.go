package assistant

import (
	"encoding/json"
	"strings"
)

// UserConfirmed accepts only the latest user turn; a topic change revokes an old approval.
func UserConfirmed(transcript []Message, claim string) bool {
	claim = strings.TrimSpace(claim)
	if claim == "" {
		return false
	}
	for i := len(transcript) - 1; i >= 0; i-- {
		if transcript[i].Role != RoleUser {
			continue
		}
		var answer userContent
		if json.Unmarshal(transcript[i].Content, &answer) != nil {
			return false
		}
		if strings.TrimSpace(answer.Text) == claim {
			return true
		}
		switch strings.ToLower(strings.Trim(strings.TrimSpace(answer.Text), ".!")) {
		case "yes", "да", "sim", "sí", "oui", "ja":
		default:
			return false
		}
		var pending []string
		for j := i - 1; j >= 0 && transcript[j].Role != RoleUser; j-- {
			if transcript[j].Role != RoleAssistant {
				continue
			}
			var content assistantContent
			if json.Unmarshal(transcript[j].Content, &content) != nil {
				return false
			}
			for _, call := range content.ToolCalls {
				if call.Name != "request_confirmation" {
					continue
				}
				var request struct {
					Claim string `json:"claim"`
				}
				if json.Unmarshal([]byte(call.Arguments), &request) != nil {
					return false
				}
				pending = append(pending, strings.TrimSpace(request.Claim))
			}
		}
		return len(pending) == 1 && pending[0] == claim
	}
	return false
}
