package domain

import "encoding/json"

type Engine string

const (
	EngineBasic Engine = "basic"
)

type Status string

const (
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
)

type InvoiceRequest struct {
	TemplateUUID string          `json:"template_uuid"`
	Engine       Engine          `json:"engine"`
	Payload      json.RawMessage `json:"payload"`
}

type Invoice struct {
	ID           string          `json:"id"`
	TemplateUUID string          `json:"template_uuid"`
	Engine       Engine          `json:"engine"`
	Payload      json.RawMessage `json:"payload"`
	Status       Status          `json:"status"`
	Error        string          `json:"error,omitempty"`
	PDFPath      string          `json:"pdf_path,omitempty"`
}

type StatusResponse struct {
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
	URL    string `json:"url,omitempty"`
}

type CreateResponse struct {
	InvoiceID string `json:"invoice_id"`
	StatusURL string `json:"status_url"`
}
