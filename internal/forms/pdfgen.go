package forms

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/signintech/gopdf"
)

//go:embed templates/7002.pdf
var template7002 []byte

//go:embed templates/7000.pdf
var template7000 []byte

//go:embed fonts/Rubik-Regular.ttf
var rubikFontTTF []byte

var formTemplates = map[string][]byte{
	"7002": template7002,
	"7000": template7000,
}

var formPageCounts = map[string]int{
	"7002": 5,
	"7000": 5,
}

// FieldPos describes where to stamp a single form field on the PDF page.
// Coordinates are in PDF points from the bottom-left corner of the page.
// Page size for both forms: 595.27 × 841.89 pts (A4).
//
// Calibration: run POST /api/forms/pdf with query ?debug=1 to receive the PDF
// with a 50pt grid overlay that makes it easy to measure coordinates visually.
type FieldPos struct {
	Page     int     // 1-based page number
	X, Y     float64 // baseline origin in PDF points from bottom-left
	FontSize float64
}

// fieldPositions maps formID → fieldKey → stamping position.
var fieldPositions = map[string]map[string]FieldPos{
	"7002": {
		// Section א — Personal details table
		"seller_name":    {Page: 1, X: 453, Y: 624, FontSize: 10},
		"seller_id":      {Page: 1, X: 325, Y: 624, FontSize: 10},
		"seller_address": {Page: 1, X: 130, Y: 624, FontSize: 9},
		// Buyer row (~28 pts below seller)
		"buyer_name":    {Page: 1, X: 453, Y: 596, FontSize: 10},
		"buyer_id":      {Page: 1, X: 325, Y: 596, FontSize: 10},
		"buyer_address": {Page: 1, X: 130, Y: 596, FontSize: 9},
		// Property details
		"gush":      {Page: 1, X: 362, Y: 503, FontSize: 10},
		"helka":     {Page: 1, X: 257, Y: 503, FontSize: 10},
		"tat_helka": {Page: 1, X: 152, Y: 503, FontSize: 10},
		// Consideration and deal date
		"consideration": {Page: 1, X: 195, Y: 453, FontSize: 10},
		"deal_date":     {Page: 1, X: 45, Y: 503, FontSize: 9},
		"right_type":    {Page: 1, X: 385, Y: 453, FontSize: 9},
	},
	"7000": {
		"seller_name":             {Page: 1, X: 448, Y: 622, FontSize: 10},
		"seller_id":               {Page: 1, X: 308, Y: 622, FontSize: 10},
		"original_purchase_date":  {Page: 1, X: 466, Y: 530, FontSize: 9},
		"original_purchase_value": {Page: 1, X: 258, Y: 530, FontSize: 9},
		"sale_value":              {Page: 1, X: 130, Y: 530, FontSize: 9},
		"improvements":            {Page: 1, X: 388, Y: 474, FontSize: 9},
		"exemption_type":          {Page: 1, X: 388, Y: 452, FontSize: 9},
	},
}

const pageW = 595.27
const pageH = 841.89

var (
	fontOnce sync.Once
	fontPath string
	fontErr  error
)

// ensureFont writes the embedded Rubik TTF to a temp file once per process.
func ensureFont() (string, error) {
	fontOnce.Do(func() {
		dir, err := os.MkdirTemp("", "pdffonts-*")
		if err != nil {
			fontErr = fmt.Errorf("pdfgen: font tmpdir: %w", err)
			return
		}
		p := filepath.Join(dir, "Rubik-Regular.ttf")
		if err := os.WriteFile(p, rubikFontTTF, 0o600); err != nil {
			fontErr = fmt.Errorf("pdfgen: write font: %w", err)
			return
		}
		fontPath = p
	})
	return fontPath, fontErr
}

// FillPDF stamps each field value from values onto the corresponding blank
// template PDF and returns the resulting PDF bytes. Fields absent from values
// or with empty strings are silently skipped.
//
// When debug is true, a 50-point grid is overlaid on page 1 to assist with
// coordinate calibration. Grid labels are in PDF coordinate space (bottom-left).
func FillPDF(formID string, values map[string]string, debug bool) ([]byte, error) {
	tmpl, ok := formTemplates[formID]
	if !ok {
		return nil, fmt.Errorf("pdfgen: no template for form %q", formID)
	}
	positions, ok := fieldPositions[formID]
	if !ok {
		return nil, fmt.Errorf("pdfgen: no positions for form %q", formID)
	}

	fp, err := ensureFont()
	if err != nil {
		return nil, err
	}

	totalPages := formPageCounts[formID]
	if totalPages <= 0 {
		totalPages = 1
	}

	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := pdf.AddTTFFont("Rubik", fp); err != nil {
		return nil, fmt.Errorf("pdfgen: add font: %w", err)
	}

	for page := 1; page <= totalPages; page++ {
		rs := bytes.NewReader(tmpl)
		var seeker io.ReadSeeker = rs

		pdf.AddPage()
		tpl := pdf.ImportPageStream(&seeker, page, "/MediaBox")
		pdf.UseImportedTemplate(tpl, 0, 0, pageW, pageH)

		// Stamp fields on this page.
		for key, pos := range positions {
			if pos.Page != page {
				continue
			}
			val, ok := values[key]
			if !ok || val == "" {
				continue
			}
			if err := pdf.SetFont("Rubik", "", pos.FontSize); err != nil {
				return nil, fmt.Errorf("pdfgen: set font %q: %w", key, err)
			}
			// PDF y is from bottom-left; gopdf y is from top-left.
			pdf.SetXY(pos.X, pageH-pos.Y-pos.FontSize)
			if err := pdf.Text(val); err != nil {
				return nil, fmt.Errorf("pdfgen: text %q: %w", key, err)
			}
		}

		if debug && page == 1 {
			if err := drawDebugGrid(&pdf); err != nil {
				return nil, fmt.Errorf("pdfgen: debug grid: %w", err)
			}
		}
	}

	var out bytes.Buffer
	if _, err := pdf.WriteTo(&out); err != nil {
		return nil, fmt.Errorf("pdfgen: write: %w", err)
	}
	return out.Bytes(), nil
}

// drawDebugGrid overlays coordinate labels every 50 pts on the current page.
// Labels are in PDF coordinate space (bottom-left origin) for easy calibration.
func drawDebugGrid(pdf *gopdf.GoPdf) error {
	if err := pdf.SetFont("Rubik", "", 6); err != nil {
		return err
	}
	pdf.SetTextColor(180, 50, 50)
	defer pdf.SetTextColor(0, 0, 0)
	for x := 50; x <= 550; x += 50 {
		for y := 50; y <= 800; y += 50 {
			label := fmt.Sprintf("%d,%d", x, y)
			pdf.SetXY(float64(x), pageH-float64(y)-6)
			if err := pdf.Text(label); err != nil {
				return err
			}
		}
	}
	return nil
}
