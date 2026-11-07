package domain

import "encoding/json"

type Engine string

const (
	EngineBasic Engine = "basic"
	EngineHTML  Engine = "html"
)

func (e Engine) Valid() bool {
	return e == EngineBasic || e == EngineHTML
}

// InvoiceRequest is the POST /invoices body. ID is optional; when empty the
// service generates one. Engine is optional and defaults to basic.
type InvoiceRequest struct {
	ID      string          `json:"id"`
	Engine  Engine          `json:"engine"`
	Payload json.RawMessage `json:"payload"`
}

type CreateResponse struct {
	InvoiceID string `json:"invoice_id"`
	URL       string `json:"url"`
}

type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}
