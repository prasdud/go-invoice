package renderer

import (
	"bytes"
	"os"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

func TestCJKIsolation(t *testing.T) {
	dir, err := os.MkdirTemp("", "font-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.WriteFile(dir+"/DejaVuSans.ttf", dejaVuSans, 0644)

	// Test 1: ASCII only
	t.Run("ascii", func(t *testing.T) {
		pdf := gofpdf.New("P", "mm", "A4", "")
		pdf.SetFontLocation(dir)
		pdf.AddUTF8Font("DejaVu", "", "DejaVuSans.ttf")
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		pdf.Cell(40, 10, "Hello World")
		var buf bytes.Buffer
		pdf.Output(&buf)
		os.WriteFile("/tmp/test-ascii.pdf", buf.Bytes(), 0644)
		t.Log("wrote /tmp/test-ascii.pdf")
	})

	// Test 2: CJK without SplitText (direct Cell call)
	t.Run("cjk_no_split", func(t *testing.T) {
		pdf := gofpdf.New("P", "mm", "A4", "")
		pdf.SetFontLocation(dir)
		pdf.AddUTF8Font("DejaVu", "", "DejaVuSans.ttf")
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		pdf.Cell(100, 10, "\u682a\u5f0f\u4f1a\u793e \u7530\u4e2d\u5546\u4e8b")
		var buf bytes.Buffer
		pdf.Output(&buf)
		os.WriteFile("/tmp/test-cjk-nosplit.pdf", buf.Bytes(), 0644)
		t.Log("wrote /tmp/test-cjk-nosplit.pdf")
	})

	// Test 3: CJK with SplitText then MultiCell (our renderer path)
	t.Run("cjk_split_then_multicell", func(t *testing.T) {
		pdf := gofpdf.New("P", "mm", "A4", "")
		pdf.SetFontLocation(dir)
		pdf.AddUTF8Font("DejaVu", "", "DejaVuSans.ttf")
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		text := "\u682a\u5f0f\u4f1a\u793e \u7530\u4e2d\u5546\u4e8b"
		lines := pdf.SplitText(text, 100)
		for _, line := range lines {
			pdf.MultiCell(100, 5, line, "", "L", false)
		}
		t.Log("SplitText lines:", lines)
		var buf bytes.Buffer
		pdf.Output(&buf)
		os.WriteFile("/tmp/test-cjk-split.pdf", buf.Bytes(), 0644)
		t.Log("wrote /tmp/test-cjk-split.pdf")
	})

	// Test 4: Umlauts
	t.Run("umlauts", func(t *testing.T) {
		pdf := gofpdf.New("P", "mm", "A4", "")
		pdf.SetFontLocation(dir)
		pdf.AddUTF8Font("DejaVu", "", "DejaVuSans.ttf")
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		pdf.Cell(40, 10, "M\u00fcnchen")
		var buf bytes.Buffer
		pdf.Output(&buf)
		os.WriteFile("/tmp/test-umlauts.pdf", buf.Bytes(), 0644)
		t.Log("wrote /tmp/test-umlauts.pdf")
	})
}
