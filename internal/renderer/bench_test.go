package renderer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func benchCase(b *testing.B, name string) {
	b.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "payloads.json"))
	if err != nil {
		b.Fatalf("read test data: %v", err)
	}
	var cases []testCase
	if err := json.Unmarshal(data, &cases); err != nil {
		b.Fatalf("parse test data: %v", err)
	}
	var tc *testCase
	for i := range cases {
		if cases[i].Name == name {
			tc = &cases[i]
			break
		}
	}
	if tc == nil {
		b.Fatalf("case %q not found", name)
	}

	opts := Options{CompanyName: tc.Company, PrimaryColor: tc.Color}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Generate(opts, tc.Payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateBasic(b *testing.B)     { benchCase(b, "standard_us") }
func BenchmarkGenerateManyItems(b *testing.B) { benchCase(b, "many_items") }
func BenchmarkGenerateChrome(b *testing.B)    { benchCase(b, "international_unicode") }
