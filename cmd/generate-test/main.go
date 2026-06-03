package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/prasdud/go-invoice/internal/renderer"
)

type testCase struct {
	Name    string          `json:"name"`
	Company string          `json:"company"`
	Color   [3]int          `json:"color"`
	Payload json.RawMessage `json:"payload"`
}

func main() {
	data, err := os.ReadFile(filepath.Join("testdata", "payloads.json"))
	if err != nil {
		log.Fatalf("read testdata: %v", err)
	}

	var cases []testCase
	if err := json.Unmarshal(data, &cases); err != nil {
		log.Fatalf("parse testdata: %v", err)
	}

	os.MkdirAll("output", 0755)

	for _, tc := range cases {
		log.Printf("generating %s...", tc.Name)

		pdf, err := renderer.Generate(renderer.Options{
			CompanyName:  tc.Company,
			PrimaryColor: tc.Color,
		}, tc.Payload)
		if err != nil {
			log.Printf("  FAIL: %v", err)
			continue
		}

		outPath := filepath.Join("output", tc.Name+".pdf")
		if err := os.WriteFile(outPath, pdf, 0644); err != nil {
			log.Printf("  write error: %v", err)
			continue
		}

		fmt.Printf("  %s (%d bytes)\n", outPath, len(pdf))
	}

	log.Println("done")
}
