package renderer

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/prasdud/go-invoice/internal/renderer/templates"
)

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
	Logo         template.URL
}

func logoDataURI(l *Logo) template.URL {
	if l == nil {
		return ""
	}
	return template.URL(l.DataURI)
}

func generateHTML(parent context.Context, opts Options, payload json.RawMessage) ([]byte, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}

	hd := buildHTMLData(data, opts)

	var buf strings.Builder
	theme := opts.Theme
	if theme.HTML == nil {
		theme = templates.Default()
	}
	if err := theme.HTML.Execute(&buf, hd); err != nil {
		return nil, fmt.Errorf("renderer: template: %w", err)
	}

	tabCtx, cancelTab, err := htmlPool.tab(parent)
	if err != nil {
		return nil, err
	}
	defer cancelTab()

	tabCtx, cancelTimeout := context.WithTimeout(tabCtx, 60*time.Second)
	defer cancelTimeout()

	var pdfData []byte
	err = chromedp.Run(tabCtx,
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
		htmlPool.failed()
		return nil, fmt.Errorf("renderer: chromedp: %w", err)
	}

	return pdfData, nil
}

// buildHTMLData maps the raw payload and render options into the theme context.
func buildHTMLData(data map[string]json.RawMessage, opts Options) htmlData {
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

	return htmlData{
		CompanyName:  sanitize(opts.CompanyName),
		PrimaryColor: fmt.Sprintf("#%02x%02x%02x", opts.PrimaryColor[0], opts.PrimaryColor[1], opts.PrimaryColor[2]),
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
		Logo:         logoDataURI(opts.Logo),
	}
}

// browserPool keeps one Chromium process per replica. Every render gets a fresh
// tab derived from the primary context that owns the browser; the browser is
// relaunched after a failure.
var htmlPool browserPool

type browserPool struct {
	mu      sync.Mutex
	primary context.Context
	stop    context.CancelFunc
	dead    bool
}

func (p *browserPool) tab(parent context.Context) (context.Context, context.CancelFunc, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.primary == nil || p.dead {
		if p.stop != nil {
			p.stop()
		}
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
		)
		if path := os.Getenv("CHROME_PATH"); path != "" {
			opts = append(opts, chromedp.ExecPath(path))
		}
		allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
		primary, cancelPrimary := chromedp.NewContext(allocCtx)
		if err := chromedp.Run(primary); err != nil {
			cancelPrimary()
			cancelAlloc()
			p.primary, p.stop = nil, nil
			return nil, nil, fmt.Errorf("renderer: start browser: %w", err)
		}
		p.primary = primary
		p.stop = func() {
			cancelPrimary()
			cancelAlloc()
		}
		p.dead = false
	}

	ctx, cancel := chromedp.NewContext(p.primary)
	// Close the tab when the caller's request context ends.
	stopTab := context.AfterFunc(parent, cancel)
	return ctx, func() {
		stopTab()
		cancel()
	}, nil
}

// failed marks the shared browser as unusable; the next render relaunches it.
func (p *browserPool) failed() {
	p.mu.Lock()
	p.dead = true
	p.mu.Unlock()
}
