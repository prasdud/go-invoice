package renderer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"

	"github.com/jung-kurt/gofpdf"
)

// maxLogoBytes caps the logo size; the logo is embedded in every invoice, so
// this keeps PDFs (and uploads) lean.
const maxLogoBytes = 1 << 20

// Logo is a validated company logo, loaded once at startup.
type Logo struct {
	Bytes   []byte
	Type    string // gofpdf image type: "PNG" or "JPG"
	DataURI string // prebuilt data URI for the html theme
}

// LoadLogo reads and validates a logo file: PNG or JPEG, at most 1 MiB.
func LoadLogo(path string) (*Logo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("renderer: read logo: %w", err)
	}
	if len(data) > maxLogoBytes {
		return nil, fmt.Errorf("renderer: logo exceeds %d bytes", maxLogoBytes)
	}

	var mime, imgType string
	switch http.DetectContentType(data) {
	case "image/png":
		mime, imgType = "image/png", "PNG"
	case "image/jpeg":
		mime, imgType = "image/jpeg", "JPG"
	default:
		return nil, fmt.Errorf("renderer: logo must be PNG or JPEG")
	}

	return &Logo{
		Bytes:   data,
		Type:    imgType,
		DataURI: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data),
	}, nil
}

// registerLogo registers the image with the document and returns its display
// size: scaled to fit the header (max 12mm tall, 50mm wide, aspect kept).
func registerLogo(pdf *gofpdf.Fpdf, logo *Logo) (w, h float64) {
	info := pdf.RegisterImageOptionsReader("logo", gofpdf.ImageOptions{ImageType: logo.Type}, bytes.NewReader(logo.Bytes))
	if info == nil {
		return 0, 0
	}
	iw, ih := info.Extent()
	if iw <= 0 || ih <= 0 {
		return 0, 0
	}
	const maxH, maxW = 12.0, 50.0
	h = maxH
	w = iw * h / ih
	if w > maxW {
		w = maxW
		h = ih * w / iw
	}
	return w, h
}
