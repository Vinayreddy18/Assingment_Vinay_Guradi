package domain

import "testing"

func TestFormatINR(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "₹0"},
		{5, "₹5"},
		{999, "₹999"},
		{1000, "₹1,000"},
		{10000, "₹10,000"},
		{100000, "₹1,00,000"},
		{250000, "₹2,50,000"},
		{1200000, "₹12,00,000"},
		{10000000, "₹1,00,00,000"},
		{-50000, "₹-50,000"},
	}
	for _, c := range cases {
		if got := FormatINR(c.in); got != c.want {
			t.Errorf("FormatINR(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCategoryValid(t *testing.T) {
	valid := []Category{
		CategoryLoanDefault, CategoryChequeBounce, CategoryEcommerce,
		CategoryRentTenancy, CategoryServiceDeficiency, CategoryGeneric,
	}
	for _, c := range valid {
		if !c.Valid() {
			t.Errorf("expected %q to be valid", c)
		}
	}
	if Category("nonsense").Valid() {
		t.Error("expected an unknown category to be invalid")
	}
}
