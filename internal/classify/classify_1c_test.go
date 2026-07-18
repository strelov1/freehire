package classify

import "testing"

// TestParse1C covers 1С category resolution: a title whose only role signal is 1С reads as backend
// (server-side enterprise dev), but a more specific role word in the title still wins because the
// 1С aliases sit last in the precedence table.
func TestParse1C(t *testing.T) {
	cases := []struct {
		title   string
		wantCat string
	}{
		{"Программист 1С", "backend"},
		{"1С-разработчик", "backend"},
		{"1C Developer", "backend"},
		{"Аналитик 1С", "data_analytics"}, // analyst wins over 1С→backend
		{"Тестировщик 1С", "qa"},          // qa wins over 1С→backend
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCat {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCat)
		}
	}
}
