package renderer

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/jung-kurt/gofpdf"
)

//go:embed DejaVuSans.ttf
var dejaVuSans []byte

var fontPath string
var fontDirPath string
var fontOnce sync.Once

func fontDir() string {
	fontOnce.Do(func() {
		dir, err := os.MkdirTemp("", "go-invoice-fonts")
		if err != nil {
			panic("renderer: cannot create temp font dir: " + err.Error())
		}
		fp := dir + "/DejaVuSans.ttf"
		if err := os.WriteFile(fp, dejaVuSans, 0644); err != nil {
			os.RemoveAll(dir)
			panic("renderer: cannot write temp font: " + err.Error())
		}
		fontDirPath = dir
		fontPath = fp
	})
	return fontDirPath
}

type Options struct {
	CompanyName  string
	PrimaryColor [3]int
}

type item struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	Rate        float64 `json:"rate"`
	Amount      float64 `json:"amount"`
}

func Generate(opts Options, payload json.RawMessage) ([]byte, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("renderer: parse payload: %w", err)
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetFontLocation(fontDir())
	pdf.AddUTF8Font("DejaVu", "", "DejaVuSans.ttf")
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.SetAutoPageBreak(true, 15)

	pageW, _ := pdf.GetPageSize()
	margin := pdf.GetX()
	width := pageW - 2*margin

	row := func(label string, value string) {
		if label == "" || value == "" {
			return
		}
		pdf.SetFont("DejaVu", "", 10)
		pdf.CellFormat(45, 6, label+":", "", 0, "L", false, 0, "")
		pdf.SetFont("DejaVu", "", 10)
		pdf.MultiCell(width-45, 6, wrapText(pdf, value, width-45), "", "L", false)
	}

	line := func() {
		y := pdf.GetY() + 2
		pdf.Line(margin, y, margin+width, y)
		pdf.Ln(3)
	}

	if opts.PrimaryColor[0] == 0 && opts.PrimaryColor[1] == 0 && opts.PrimaryColor[2] == 0 {
		opts.PrimaryColor = [3]int{40, 60, 120}
	}

	r, g, b := opts.PrimaryColor[0], opts.PrimaryColor[1], opts.PrimaryColor[2]
	pdf.SetFillColor(r, g, b)
	pdf.SetTextColor(255, 255, 255)
	pdf.Rect(margin, 10, width, 18, "F")
	pdf.SetFont("DejaVu", "", 16)
	pdf.SetXY(margin+4, 12)
	pdf.CellFormat(width-8, 12, sanitize(opts.CompanyName), "", 0, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetY(32)

	row("Invoice #", strVal(data, "invoice_number"))
	row("Date", strVal(data, "date"))
	row("Due Date", strVal(data, "due_date"))
	pdf.Ln(4)

	pdf.SetFont("DejaVu", "", 11)
	pdf.CellFormat(0, 6, "Bill To:", "", 1, "L", false, 0, "")
	billTo := strVal(data, "bill_to_name")
	if billTo == "" {
		billTo = strVal(data, "bill_to")
	}
	if billTo != "" {
		pdf.SetFont("DejaVu", "", 10)
		pdf.CellFormat(0, 6, sanitize(billTo), "", 1, "L", false, 0, "")
	}
	billAddr := strVal(data, "bill_to_address")
	if billAddr != "" {
		pdf.SetFont("DejaVu", "", 10)
		pdf.CellFormat(0, 6, sanitize(billAddr), "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)
	line()

	colW := []float64{80, 30, 30, 30}
	headers := []string{"Description", "Qty", "Rate", "Amount"}
	pdf.SetFont("DejaVu", "", 10)
	pdf.SetFillColor(r, g, b)
	pdf.SetTextColor(255, 255, 255)
	for i, h := range headers {
		pdf.CellFormat(colW[i], 8, h, "", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetTextColor(0, 0, 0)

	items := getItems(data)
	var subtotal float64
	for _, it := range items {
		if it.Description == "" {
			continue
		}
		amount := it.Amount
		if amount == 0 {
			amount = it.Quantity * it.Rate
		}
		subtotal += amount

		rowH := itemRowHeight(pdf, sanitize(it.Description), colW[0])
		pdf.SetFont("DejaVu", "", 9)
		x := pdf.GetX()
		y := pdf.GetY()
		pdf.Rect(x, y, colW[0], rowH, "D")
		pdf.MultiCell(colW[0], 5, sanitize(it.Description), "", "L", false)

		pdf.SetXY(x+colW[0], y)
		pdf.Rect(x+colW[0], y, colW[1], rowH, "D")
		pdf.CellFormat(colW[1], rowH, strconv.FormatFloat(it.Quantity, 'f', 2, 64), "", 0, "R", false, 0, "")

		pdf.SetXY(x+colW[0]+colW[1], y)
		pdf.Rect(x+colW[0]+colW[1], y, colW[2], rowH, "D")
		pdf.CellFormat(colW[2], rowH, strconv.FormatFloat(it.Rate, 'f', 2, 64), "", 0, "R", false, 0, "")

		pdf.SetXY(x+colW[0]+colW[1]+colW[2], y)
		pdf.Rect(x+colW[0]+colW[1]+colW[2], y, colW[3], rowH, "D")
		pdf.CellFormat(colW[3], rowH, strconv.FormatFloat(amount, 'f', 2, 64), "", 1, "R", false, 0, "")
	}
	line()

	taxRate := floatVal(data, "tax_rate")
	tax := subtotal * taxRate / 100
	total := subtotal + tax

	totalRow := func(label string, val float64) {
		pdf.SetFont("DejaVu", "", 10)
		pdf.CellFormat(140, 6, label, "", 0, "R", false, 0, "")
		pdf.SetFont("DejaVu", "", 10)
		pdf.CellFormat(30, 6, fmt.Sprintf("%.2f", val), "", 1, "R", false, 0, "")
	}

	totalRow("Subtotal:", subtotal)
	if taxRate > 0 {
		totalRow(fmt.Sprintf("Tax (%.0f%%):", taxRate), tax)
	}
	pdf.SetFont("DejaVu", "", 12)
	pdf.CellFormat(140, 8, "Total:", "", 0, "R", false, 0, "")
	pdf.CellFormat(30, 8, fmt.Sprintf("%.2f", total), "", 1, "R", false, 0, "")
	pdf.Ln(6)

	notes := strVal(data, "notes")
	if notes != "" {
		pdf.SetFont("DejaVu", "", 9)
		pdf.MultiCell(width, 5, sanitize(notes), "", "L", false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("renderer: output: %w", err)
	}
	return buf.Bytes(), nil
}

func sanitize(s string) string {
	s = stripEmoji(s)
	if len(s) > 5000 {
		s = s[:5000]
	}
	return s
}

func stripEmoji(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r <= 0xFFFF {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func wrapText(pdf *gofpdf.Fpdf, text string, width float64) string {
	lines := pdf.SplitText(text, width)
	return strings.Join(lines, "\n")
}

func itemRowHeight(pdf *gofpdf.Fpdf, desc string, colW float64) float64 {
	lines := pdf.SplitText(desc, colW-2)
	h := float64(len(lines)) * 5
	if h < 8 {
		h = 8
	}
	return h
}

func strVal(data map[string]json.RawMessage, key string) string {
	v, ok := data[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return stripEmoji(s)
}

func floatVal(data map[string]json.RawMessage, key string) float64 {
	v, ok := data[key]
	if !ok {
		return 0
	}
	var f float64
	if err := json.Unmarshal(v, &f); err != nil {
		return 0
	}
	return math.Round(f*100) / 100
}

func getItems(data map[string]json.RawMessage) []item {
	v, ok := data["items"]
	if !ok {
		return nil
	}
	var items []item
	if err := json.Unmarshal(v, &items); err != nil {
		return nil
	}
	return items
}
