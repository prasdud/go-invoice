package renderer

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

//go:embed invoice.html
var invoiceHTML string

var htmlTmpl = template.Must(template.New("invoice").
	Funcs(template.FuncMap{
		"f2s": func(f float64) string { return fmt.Sprintf("%.2f", f) },
	}).Parse(invoiceHTML))

type htmlItem struct {
	Description string
	Quantity    string
	Rate        string
	Amount      string
}

type htmlData struct {
	CompanyName  string
	PrimaryColor string
	InvoiceNum   string
	Date         string
	DueDate      string
	BillTo       string
	BillToAddr   string
	Items        []htmlItem
	Subtotal     string
	TaxRate      float64
	Tax          string
	Total        string
	Notes        string
}

func generateChrome(opts Options, payload json.RawMessage) ([]byte, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}

	color := fmt.Sprintf("#%02x%02x%02x", opts.PrimaryColor[0], opts.PrimaryColor[1], opts.PrimaryColor[2])

	billTo := strVal(data, "bill_to_name")
	if billTo == "" {
		billTo = strVal(data, "bill_to")
	}

	items := getItems(data)
	var htmlItems []htmlItem
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
		htmlItems = append(htmlItems, htmlItem{
			Description: sanitize(it.Description),
			Quantity:    fmt.Sprintf("%.2f", it.Quantity),
			Rate:        fmt.Sprintf("%.2f", it.Rate),
			Amount:      fmt.Sprintf("%.2f", amount),
		})
	}

	taxRate := floatVal(data, "tax_rate")
	tax := subtotal * taxRate / 100
	total := subtotal + tax

	hd := htmlData{
		CompanyName:  sanitize(opts.CompanyName),
		PrimaryColor: color,
		InvoiceNum:   strVal(data, "invoice_number"),
		Date:         strVal(data, "date"),
		DueDate:      strVal(data, "due_date"),
		BillTo:       sanitize(billTo),
		BillToAddr:   sanitize(strVal(data, "bill_to_address")),
		Items:        htmlItems,
		Subtotal:     fmt.Sprintf("%.2f", subtotal),
		TaxRate:      math.Round(taxRate*100) / 100,
		Tax:          fmt.Sprintf("%.2f", tax),
		Total:        fmt.Sprintf("%.2f", total),
		Notes:        sanitize(strVal(data, "notes")),
	}

	var buf strings.Builder
	if err := htmlTmpl.Execute(&buf, hd); err != nil {
		return nil, fmt.Errorf("renderer: template: %w", err)
	}

	optsC := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), optsC...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var pdfData []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(tree.Frame.ID, buf.String()).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdfData, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(8.27).
				WithPaperHeight(11.69).
				WithMarginTop(0).
				WithMarginBottom(0).
				WithMarginLeft(0).
				WithMarginRight(0).
				Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("renderer: chromedp: %w", err)
	}

	return pdfData, nil
}
