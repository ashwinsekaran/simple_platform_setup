package ent

import "encoding/json"

type Event struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type EventRecord struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Payload          json.RawMessage `json:"payload"`
	ProcessingStatus string          `json:"processing_status"`
	ProcessingResult string          `json:"processing_result,omitempty"`
}
