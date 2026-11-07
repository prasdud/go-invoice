package renderer

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prasdud/go-invoice/internal/domain"
	"github.com/prasdud/go-invoice/internal/renderer/templates"
)

func testImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 56, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 56; x++ {
			img.Set(x, y, color.RGBA{R: 40, G: 72, B: 120, A: 255})
		}
	}
	return img
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadLogoPNG(t *testing.T) {
	path := writeTempFile(t, "logo.png", testPNG(t))
	logo, err := LoadLogo(path)
	if err != nil {
		t.Fatalf("LoadLogo: %v", err)
	}
	if logo.Type != "PNG" {
		t.Fatalf("type = %q, want PNG", logo.Type)
	}
	if !strings.HasPrefix(logo.DataURI, "data:image/png;base64,") {
		t.Fatalf("data uri = %q", logo.DataURI)
	}
}

func TestLoadLogoJPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), nil); err != nil {
		t.Fatal(err)
	}
	path := writeTempFile(t, "logo.jpg", buf.Bytes())
	logo, err := LoadLogo(path)
	if err != nil {
		t.Fatalf("LoadLogo: %v", err)
	}
	if logo.Type != "JPG" {
		t.Fatalf("type = %q, want JPG", logo.Type)
	}
	if !strings.HasPrefix(logo.DataURI, "data:image/jpeg;base64,") {
		t.Fatalf("data uri = %q", logo.DataURI)
	}
}

func TestLoadLogoRejects(t *testing.T) {
	if _, err := LoadLogo(filepath.Join(t.TempDir(), "missing.png")); err == nil {
		t.Fatal("missing file accepted")
	}
	if _, err := LoadLogo(writeTempFile(t, "notes.txt", []byte("not an image"))); err == nil {
		t.Fatal("text file accepted")
	}
	png := testPNG(t)
	big := writeTempFile(t, "big.png", bytes.Repeat(png, maxLogoBytes/len(png)+2))
	if _, err := LoadLogo(big); err == nil {
		t.Fatal("oversized logo accepted")
	}
}

func TestGenerateBasicWithLogo(t *testing.T) {
	logo := &Logo{Bytes: testPNG(t), Type: "PNG", DataURI: "data:image/png;base64,x"}
	pdf, err := Generate(context.Background(), domain.EngineBasic,
		Options{CompanyName: "Acme Corp", Logo: logo},
		json.RawMessage(`{"invoice_number":"INV-1"}`))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !bytes.Contains(pdf, []byte("/Subtype /Image")) {
		t.Fatal("logo image missing from basic PDF")
	}
}

func TestGenerateBasicWithoutLogo(t *testing.T) {
	pdf, err := Generate(context.Background(), domain.EngineBasic,
		Options{CompanyName: "Acme Corp"},
		json.RawMessage(`{"invoice_number":"INV-1"}`))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if bytes.Contains(pdf, []byte("/Subtype /Image")) {
		t.Fatal("unexpected image in basic PDF without logo")
	}
}

func TestHTMLThemeLogo(t *testing.T) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{"invoice_number":"INV-1"}`), &data); err != nil {
		t.Fatal(err)
	}

	withLogo := buildHTMLData(data, Options{
		CompanyName: "Acme",
		Logo:        &Logo{DataURI: "data:image/png;base64,AAAA"},
	})
	var buf strings.Builder
	if err := templates.Default().HTML.Execute(&buf, withLogo); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `src="data:image/png;base64,AAAA"`) {
		t.Fatalf("logo missing from rendered theme")
	}

	withoutLogo := buildHTMLData(data, Options{CompanyName: "Acme"})
	buf.Reset()
	if err := templates.Default().HTML.Execute(&buf, withoutLogo); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<img") {
		t.Fatal("unexpected img tag without logo")
	}
}
