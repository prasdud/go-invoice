package validate

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/prasdud/go-invoice/internal/domain"
)

// Error carries a machine-readable code so the API layer can map failures to
// HTTP status codes without parsing messages.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9_-]{22,64}$`)

// ValidID reports whether s is a well-formed invoice id.
func ValidID(s string) bool {
	return idRe.MatchString(s)
}

const MaxItems = 200

// InvoiceRequest validates the request envelope. r.Engine must already be
// defaulted by the caller.
func InvoiceRequest(r domain.InvoiceRequest) error {
	if r.ID != "" && !idRe.MatchString(r.ID) {
		return fail("invalid_id", "id must match [A-Za-z0-9_-]{22,64}")
	}
	if !r.Engine.Valid() {
		return fail("unknown_engine", "engine must be %q or %q", domain.EngineBasic, domain.EngineHTML)
	}
	if len(r.Payload) == 0 {
		return fail("invalid_payload", "payload is required")
	}
	return payload(r.Payload)
}

var stringFields = []string{
	"invoice_number", "date", "due_date",
	"bill_to", "bill_to_name", "bill_to_address", "notes",
}

func payload(raw json.RawMessage) error {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil || data == nil {
		return fail("invalid_payload", "payload must be a JSON object")
	}

	for _, f := range stringFields {
		if v, ok := data[f]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				return fail("invalid_payload", "%s must be a string", f)
			}
		}
	}

	if v, ok := data["tax_rate"]; ok {
		var n float64
		if err := json.Unmarshal(v, &n); err != nil {
			return fail("invalid_payload", "tax_rate must be a number")
		}
	}

	v, ok := data["items"]
	if !ok {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(v, &items); err != nil {
		return fail("invalid_payload", "items must be an array")
	}
	if len(items) > MaxItems {
		return fail("invalid_payload", "items must not exceed %d entries", MaxItems)
	}
	for i, raw := range items {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			return fail("invalid_payload", "items[%d] must be an object", i)
		}
		if d, ok := item["description"]; ok {
			var s string
			if err := json.Unmarshal(d, &s); err != nil {
				return fail("invalid_payload", "items[%d].description must be a string", i)
			}
		}
		for _, f := range []string{"quantity", "rate", "amount"} {
			if v, ok := item[f]; ok {
				var n float64
				if err := json.Unmarshal(v, &n); err != nil {
					return fail("invalid_payload", "items[%d].%s must be a number", i, f)
				}
			}
		}
	}
	return nil
}
