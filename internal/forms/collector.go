// Package forms defines the government forms supported in the MVP and a
// small state machine that tracks which required fields have been collected.
//
// The *actual* back-and-forth with the user is driven by Claude through the
// system prompt; this package exists so the Go layer can:
//
//   - enumerate supported forms to the frontend (for quick-action chips)
//   - validate submitted values
//   - render a human-readable Hebrew summary for confirmation
package forms

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// FieldType constrains which kind of value a field expects.
type FieldType string

const (
	FieldText FieldType = "text"
	FieldID   FieldType = "id"   // Israeli teudat zehut (9 digits)
	FieldDate FieldType = "date" // free-form Hebrew date string; not strictly validated
	FieldNum  FieldType = "number"
)

// Field describes a required input on a form.
type Field struct {
	Key         string    // stable machine key (ascii)
	LabelHE     string    // Hebrew label shown to user
	Type        FieldType
	Required    bool
}

// Form is the static definition of a supported government form.
type Form struct {
	ID      string  // stable machine id
	NameHE  string  // Hebrew display name
	Fields  []Field
}

// SupportedForms are the three MVP forms from the spec.
var SupportedForms = []Form{
	{
		ID:     "7002",
		NameHE: "טופס 7002 — הצהרה על מכירת/רכישת זכות במקרקעין",
		Fields: []Field{
			{Key: "seller_name", LabelHE: "שם המוכר", Type: FieldText, Required: true},
			{Key: "seller_id", LabelHE: "ת\"ז המוכר", Type: FieldID, Required: true},
			{Key: "seller_address", LabelHE: "כתובת המוכר", Type: FieldText, Required: true},
			{Key: "buyer_name", LabelHE: "שם הקונה", Type: FieldText, Required: true},
			{Key: "buyer_id", LabelHE: "ת\"ז הקונה", Type: FieldID, Required: true},
			{Key: "buyer_address", LabelHE: "כתובת הקונה", Type: FieldText, Required: true},
			{Key: "gush", LabelHE: "גוש", Type: FieldText, Required: true},
			{Key: "helka", LabelHE: "חלקה", Type: FieldText, Required: true},
			{Key: "tat_helka", LabelHE: "תת-חלקה", Type: FieldText, Required: false},
			{Key: "consideration", LabelHE: "תמורה (בש\"ח)", Type: FieldNum, Required: true},
			{Key: "deal_date", LabelHE: "תאריך העסקה", Type: FieldDate, Required: true},
			{Key: "right_type", LabelHE: "סוג הזכות", Type: FieldText, Required: true},
		},
	},
	{
		ID:     "7000",
		NameHE: "טופס 7000 — הצהרה בעניין מס שבח",
		Fields: []Field{
			{Key: "seller_name", LabelHE: "שם המוכר", Type: FieldText, Required: true},
			{Key: "seller_id", LabelHE: "ת\"ז המוכר", Type: FieldID, Required: true},
			{Key: "original_purchase_date", LabelHE: "תאריך רכישה מקורית", Type: FieldDate, Required: true},
			{Key: "original_purchase_value", LabelHE: "שווי רכישה מקורית", Type: FieldNum, Required: true},
			{Key: "sale_value", LabelHE: "שווי מכירה", Type: FieldNum, Required: true},
			{Key: "improvements", LabelHE: "שיפורים שבוצעו (אם יש)", Type: FieldText, Required: false},
			{Key: "exemption_type", LabelHE: "סוג פטור (אם רלוונטי)", Type: FieldText, Required: false},
		},
	},
	{
		ID:     "tabu_registration",
		NameHE: "בקשה לרישום עסקה בטאבו",
		Fields: []Field{
			{Key: "parties", LabelHE: "פרטי הצדדים", Type: FieldText, Required: true},
			{Key: "power_of_attorney", LabelHE: "ייפוי כוח (מספר / אישור)", Type: FieldText, Required: true},
			{Key: "tabu_nasach", LabelHE: "נסח טאבו עדכני", Type: FieldText, Required: true},
			{Key: "tax_payment", LabelHE: "אסמכתאות תשלום מס", Type: FieldText, Required: true},
		},
	},
}

// FindForm returns the Form with the given id or (Form{}, false).
func FindForm(id string) (Form, bool) {
	for _, f := range SupportedForms {
		if f.ID == id {
			return f, true
		}
	}
	return Form{}, false
}

// Collector tracks which fields have been filled in for a single form session.
type Collector struct {
	form   Form
	values map[string]string
}

// NewCollector initializes a Collector for a given form id.
func NewCollector(formID string) (*Collector, error) {
	f, ok := FindForm(formID)
	if !ok {
		return nil, fmt.Errorf("forms: unknown form id %q", formID)
	}
	return &Collector{form: f, values: map[string]string{}}, nil
}

// Form returns the underlying form definition.
func (c *Collector) Form() Form { return c.form }

// Set validates and stores a value for the given field key.
func (c *Collector) Set(key, value string) error {
	var field *Field
	for i := range c.form.Fields {
		if c.form.Fields[i].Key == key {
			field = &c.form.Fields[i]
			break
		}
	}
	if field == nil {
		return fmt.Errorf("forms: unknown field %q for form %s", key, c.form.ID)
	}
	value = strings.TrimSpace(value)
	if err := validate(field.Type, value); err != nil {
		return err
	}
	c.values[key] = value
	return nil
}

// Get returns the stored value and whether it was set.
func (c *Collector) Get(key string) (string, bool) {
	v, ok := c.values[key]
	return v, ok
}

// MissingFields returns the keys of required fields that haven't been filled.
// Always returns a non-nil slice so JSON serialization produces [] not null.
func (c *Collector) MissingFields() []string {
	out := make([]string, 0)
	for _, f := range c.form.Fields {
		if !f.Required {
			continue
		}
		if _, ok := c.values[f.Key]; !ok {
			out = append(out, f.Key)
		}
	}
	return out
}

// NextField returns the next required unfilled field, or nil if complete.
func (c *Collector) NextField() *Field {
	for i := range c.form.Fields {
		f := &c.form.Fields[i]
		if !f.Required {
			continue
		}
		if _, ok := c.values[f.Key]; !ok {
			return f
		}
	}
	return nil
}

// IsComplete reports whether all required fields have values.
func (c *Collector) IsComplete() bool {
	return c.NextField() == nil
}

// SummaryHebrew renders a Hebrew confirmation summary of collected values.
func (c *Collector) SummaryHebrew() string {
	var b strings.Builder
	b.WriteString("סיכום — ")
	b.WriteString(c.form.NameHE)
	b.WriteString("\n\n")
	for _, f := range c.form.Fields {
		v, ok := c.values[f.Key]
		if !ok {
			if f.Required {
				b.WriteString("• " + f.LabelHE + ": [חסר]\n")
			}
			continue
		}
		b.WriteString("• " + f.LabelHE + ": " + v + "\n")
	}
	return b.String()
}

// ToJSON serializes the collected values and form metadata as JSON.
func (c *Collector) ToJSON() ([]byte, error) {
	type export struct {
		FormID   string            `json:"form_id"`
		FormName string            `json:"form_name"`
		Values   map[string]string `json:"values"`
	}
	return json.Marshal(export{
		FormID:   c.form.ID,
		FormName: c.form.NameHE,
		Values:   c.values,
	})
}

// validate enforces very light type rules. Intentionally lenient — the
// system prompt does the heavy lifting on what "looks right".
func validate(t FieldType, value string) error {
	if value == "" {
		return errors.New("ערך ריק")
	}
	switch t {
	case FieldID:
		// 9 digits, optionally with leading zeros; we just count digits.
		digits := 0
		for _, r := range value {
			if r < '0' || r > '9' {
				return fmt.Errorf("ת\"ז חייבת להכיל ספרות בלבד")
			}
			digits++
		}
		if digits != 9 {
			return fmt.Errorf("ת\"ז חייבת להיות 9 ספרות, קיבלתי %d", digits)
		}
	case FieldNum:
		// Accept digits, spaces, commas and a single dot.
		dotSeen := false
		for _, r := range value {
			switch {
			case r >= '0' && r <= '9':
			case r == ',' || r == ' ':
			case r == '.':
				if dotSeen {
					return fmt.Errorf("מספר לא תקין")
				}
				dotSeen = true
			default:
				return fmt.Errorf("מספר לא תקין")
			}
		}
	}
	return nil
}
