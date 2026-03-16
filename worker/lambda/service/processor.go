package service

import (
	"encoding/json"
	"fmt"
)

type userCreatedPayload struct {
	Name string `json:"name"`
}

func processEvent(eventType string, payload json.RawMessage) error {
	switch eventType {
	case "user.created":
		return validateUserCreated(payload)
	default:
		return nil
	}
}

func validateUserCreated(payload json.RawMessage) error {
	var input userCreatedPayload
	if err := json.Unmarshal(payload, &input); err != nil {
		return fmt.Errorf("invalid user.created payload: %w", err)
	}

	if input.Name == "" {
		return fmt.Errorf("user.created requires payload.name")
	}

	return nil
}
