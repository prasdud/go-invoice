package renderer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prasdud/go-invoice/internal/storage"
)

type testCase struct {
	Name    string          `json:"name"`
	Company string          `json:"company"`
	Color   [3]int          `json:"color"`
	Payload json.RawMessage `json:"payload"`
}

func TestGenerate(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "payloads.json"))
	if err != nil {
		t.Fatalf("read test data: %v", err)
	}

	var cases []testCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse test data: %v", err)
	}

	os.MkdirAll(filepath.Join("..", "..", "output"), 0755)

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			pdf, err := Generate(Options{
				CompanyName:  tc.Company,
				PrimaryColor: tc.Color,
			}, tc.Payload)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}

			if len(pdf) == 0 {
				t.Fatal("pdf is empty")
			}

			outPath := filepath.Join("..", "..", "output", tc.Name+".pdf")
			if err := os.WriteFile(outPath, pdf, 0644); err != nil {
				t.Fatalf("write pdf: %v", err)
			}

			t.Logf("generated %s (%d bytes)", outPath, len(pdf))
		})
	}
}

func TestGenerateEmptyPayload(t *testing.T) {
	_, err := Generate(Options{CompanyName: "Test"}, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("empty payload should not error: %v", err)
	}
}

func TestGenerateNilPayload(t *testing.T) {
	_, err := Generate(Options{CompanyName: "Test"}, json.RawMessage(`null`))
	if err != nil {
		t.Fatalf("null payload should not error: %v", err)
	}
}

func TestGenerateEmptyItems(t *testing.T) {
	_, err := Generate(Options{CompanyName: "Test"}, json.RawMessage(`{"items":[]}`))
	if err != nil {
		t.Fatalf("empty items should not error: %v", err)
	}
}

func TestGenerateMissingFields(t *testing.T) {
	payload := json.RawMessage(`{
		"invoice_number": "INV-001",
		"bill_to": "Someone"
	}`)
	pdf, err := Generate(Options{CompanyName: "Test"}, payload)
	if err != nil {
		t.Fatalf("missing fields should not error: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("pdf is empty")
	}
}

func TestGenerateUnicodeEdgeCases(t *testing.T) {
	payload := json.RawMessage(`{
		"invoice_number": "INV-💯",
		"bill_to": "Café München 株式会社",
		"items": [
			{"description": "こんにちは мир hello", "quantity": 1, "rate": 100}
		]
	}`)
	pdf, err := Generate(Options{CompanyName: "Café πr²"}, payload)
	if err != nil {
		t.Fatalf("unicode payload should not error: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("pdf is empty")
	}
	os.MkdirAll(filepath.Join("..", "..", "output"), 0755)
	os.WriteFile(filepath.Join("..", "..", "output", "unicode_edges.pdf"), pdf, 0644)
}

func TestStorageKeyFormat(t *testing.T) {
	key := storage.Key("test-uuid")
	if len(key) < 10 {
		t.Fatal("key too short")
	}
	if !strings.HasSuffix(key, ".pdf") {
		t.Fatalf("key should end with .pdf: %s", key)
	}
	// Should match year/month/uuid.pdf
	parts := strings.SplitN(key, "/", 3)
	if len(parts) != 3 {
		t.Fatalf("key format should be year/month/uuid.pdf: %s", key)
	}
}
