package templates

import (
	"strings"
	"testing"
)

func TestDefaultThemeRenders(t *testing.T) {
	th := Default()
	if th.Name != "default" {
		t.Fatalf("name = %q, want default", th.Name)
	}

	var buf strings.Builder
	err := th.HTML.Execute(&buf, map[string]any{
		"CompanyName":  "Acme Corp",
		"PrimaryColor": "#284878",
		"InvoiceNum":   "INV-1",
		"BillTo":       "Wayne Enterprises",
		"Items": []map[string]string{
			{"Description": "Widget", "Quantity": "1.00", "Rate": "2.00", "Amount": "2.00"},
		},
		"Subtotal": "2.00",
		"TaxRate":  float64(0),
		"Tax":      "0.00",
		"Total":    "2.00",
		"Notes":    "Thanks",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"Acme Corp", "INV-1", "Wayne Enterprises", "Widget", "#284878", "Thanks"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered theme missing %q", want)
		}
	}
}
