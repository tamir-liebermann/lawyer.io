package forms

import (
	"strings"
	"testing"
)

func TestFindForm(t *testing.T) {
	if _, ok := FindForm("7002"); !ok {
		t.Error("7002 should exist")
	}
	if _, ok := FindForm("nope"); ok {
		t.Error("nope should not exist")
	}
}

func TestCollector_NextField_And_Complete(t *testing.T) {
	c, err := NewCollector("tabu_registration")
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}
	// All four fields are required.
	fields := []string{"parties", "power_of_attorney", "tabu_nasach", "tax_payment"}
	if nf := c.NextField(); nf == nil || nf.Key != fields[0] {
		t.Fatalf("expected next field %s, got %+v", fields[0], nf)
	}

	for _, k := range fields {
		if err := c.Set(k, "x"); err != nil {
			t.Fatalf("Set(%q): %v", k, err)
		}
	}
	if !c.IsComplete() {
		t.Error("collector should be complete after all required fields set")
	}
	if nf := c.NextField(); nf != nil {
		t.Errorf("NextField should be nil when complete, got %+v", nf)
	}
}

func TestCollector_MissingFields(t *testing.T) {
	c, _ := NewCollector("7000")
	_ = c.Set("seller_name", "שם")
	_ = c.Set("seller_id", "123456789")
	missing := c.MissingFields()
	// 7000 has 5 required fields; we set 2 -> 3 missing.
	if len(missing) != 3 {
		t.Fatalf("expected 3 missing, got %d (%v)", len(missing), missing)
	}
}

func TestCollector_SetValidatesID(t *testing.T) {
	c, _ := NewCollector("7002")
	if err := c.Set("seller_id", "abc"); err == nil {
		t.Error("non-digit id should fail validation")
	}
	if err := c.Set("seller_id", "12345"); err == nil {
		t.Error("short id should fail validation")
	}
	if err := c.Set("seller_id", "123456789"); err != nil {
		t.Errorf("valid id rejected: %v", err)
	}
}

func TestCollector_SetValidatesNumber(t *testing.T) {
	c, _ := NewCollector("7002")
	if err := c.Set("consideration", "three"); err == nil {
		t.Error("non-numeric consideration should fail")
	}
	if err := c.Set("consideration", "1.2.3"); err == nil {
		t.Error("multiple dots should fail")
	}
	if err := c.Set("consideration", "2,500,000"); err != nil {
		t.Errorf("comma-separated number rejected: %v", err)
	}
	if err := c.Set("consideration", "1200000.50"); err != nil {
		t.Errorf("decimal number rejected: %v", err)
	}
}

func TestCollector_UnknownFieldErrors(t *testing.T) {
	c, _ := NewCollector("7000")
	if err := c.Set("nope", "x"); err == nil {
		t.Error("unknown field should error")
	}
}

func TestCollector_SummaryHebrew(t *testing.T) {
	c, _ := NewCollector("7002")
	_ = c.Set("seller_name", "ישראל ישראלי")
	_ = c.Set("seller_id", "123456789")
	sum := c.SummaryHebrew()
	if !strings.Contains(sum, "ישראל ישראלי") {
		t.Error("summary should contain seller_name value")
	}
	if !strings.Contains(sum, "טופס 7002") {
		t.Error("summary should name the form")
	}
	if !strings.Contains(sum, "[חסר]") {
		t.Error("summary should flag missing required fields")
	}
}

func TestCollector_EmptyValueErrors(t *testing.T) {
	c, _ := NewCollector("7000")
	if err := c.Set("seller_name", "   "); err == nil {
		t.Error("empty/whitespace value should error")
	}
}

func TestSupportedForms_AllHaveFields(t *testing.T) {
	for _, f := range SupportedForms {
		if f.ID == "" || f.NameHE == "" {
			t.Errorf("form missing id or name: %+v", f)
		}
		if len(f.Fields) == 0 {
			t.Errorf("form %s has no fields", f.ID)
		}
		hasRequired := false
		for _, fld := range f.Fields {
			if fld.Required {
				hasRequired = true
				break
			}
		}
		if !hasRequired {
			t.Errorf("form %s has no required fields", f.ID)
		}
	}
}
