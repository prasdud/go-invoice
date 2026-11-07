package renderer

import (
	"context"
	"encoding/json"
	"html/template"
	"os"
	"testing"

	"github.com/prasdud/go-invoice/internal/domain"
	"github.com/prasdud/go-invoice/internal/renderer/templates"
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

	pdf, err := Generate(context.Background(), domain.EngineHTML, Options{
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

func TestHTMLRecoversAfterBrowserFailure(t *testing.T) {
	payload := json.RawMessage(`{
		"bill_to": "株式会社 田中商事",
		"items": [{"description": "テスト", "quantity": 1, "rate": 1}]
	}`)
	opts := Options{CompanyName: "Recovery Test"}
	ctx := context.Background()

	if _, err := Generate(ctx, domain.EngineHTML, opts, payload); err != nil {
		t.Fatalf("first render: %v", err)
	}

	htmlPool.failed() // simulate a detected browser crash

	if _, err := Generate(ctx, domain.EngineHTML, opts, payload); err != nil {
		t.Fatalf("render after failure: %v", err)
	}
}

func TestGenerateHTMLUsesConfiguredTheme(t *testing.T) {
	bad := templates.Theme{
		Name: "bad",
		HTML: template.Must(template.New("bad").Parse(`<html><body>{{.NoSuchField}}</body></html>`)),
	}
	_, err := Generate(context.Background(), domain.EngineHTML, Options{Theme: bad}, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from the configured theme")
	}
}
