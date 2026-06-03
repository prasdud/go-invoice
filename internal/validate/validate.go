package validate

import (
	"fmt"

	"github.com/prasdud/go-invoice/internal/domain"
)

func InvoiceRequest(r domain.InvoiceRequest) error {
	if r.TemplateUUID == "" {
		return fmt.Errorf("template_uuid: required")
	}
	if r.Engine == "" {
		return fmt.Errorf("engine: required")
	}
	if r.Engine != domain.EngineBasic {
		return fmt.Errorf("engine: unknown engine %q", r.Engine)
	}
	if len(r.Payload) == 0 {
		return fmt.Errorf("payload: required")
	}
	return nil
}
