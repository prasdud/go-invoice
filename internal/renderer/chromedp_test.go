package renderer

import (
	"encoding/json"
	"os"
	"testing"
)

func TestChromeCJK(t *testing.T) {
	payload := json.RawMessage(`{
		"invoice_number": "INV-CJK-001",
		"date": "2026-06-04",
		"due_date": "2026-07-04",
		"bill_to": "株式会社田中商事",
		"bill_to_address": "〒100-0005\n東京都千代田区丸の内1-1-1",
		"items": [
			{"description": "ソフトウェアライセンス", "quantity": 5, "rate": 499.00},
			{"description": "Consultation München", "quantity": 3, "rate": 1200.00}
		],
		"tax_rate": 10,
		"notes": "お支払いは30日以内にお願いします。\nThank you for your business."
	}`)

	pdf, err := Generate(Options{
		CompanyName:  "CJK Test 株式会社",
		PrimaryColor: [3]int{180, 40, 40},
	}, payload)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(pdf) < 1000 {
		t.Fatalf("PDF too small: %d bytes", len(pdf))
	}

	os.WriteFile("../../output/chromedp_cjk_test.pdf", pdf, 0644)
	t.Logf("Generated: %d bytes", len(pdf))

	// Verify PDF signature
	if string(pdf[:4]) != "%PDF" {
		t.Fatalf("Not a PDF: starts with %q", string(pdf[:20]))
	}
}
