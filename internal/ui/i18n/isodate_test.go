package i18n

import "testing"

func TestYearOfAndFormatISODate(t *testing.T) {
	if got := YearOf("2008-06-02"); got != "2008" {
		t.Errorf("YearOf %q", got)
	}
	if got := YearOf("x2008"); got != "" {
		t.Errorf("YearOf of text %q", got)
	}
	if got := FormatISODate("2007-10-10", DateLong); got != "October 10, 2007" {
		t.Errorf("FormatISODate %q", got)
	}
	// A year alone is all there is to say; nothing says nothing.
	if got := FormatISODate("2007", DateLong); got != "2007" {
		t.Errorf("FormatISODate of a year %q", got)
	}
	if got := FormatISODate("", DateLong); got != "" {
		t.Errorf("FormatISODate of nothing %q", got)
	}
}
