package postgres

import (
	"encoding/json"
	"testing"
)

// TestMarshalJSONB - нулевой байт из ответа портала выбрасывается, потому что
// JSONB его не принимает, а текст, который сам похож на такое экранирование,
// обязан дойти целиком.
func TestMarshalJSONB(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"без нулевых байтов", "Программирование", "Программирование"},
		{"нулевой байт в строке", "Програм\x00мирование", "Программирование"},
		{"два нулевых байта подряд", "\x00\x00", ""},
		{"текст, похожий на экранирование", "a\\u0000b", "a\\u0000b"},
		{"другие escape-последовательности", "a\nb\"c\\d", "a\nb\"c\\d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := marshalJSONB(map[string]string{"title": tt.title})
			if err != nil {
				t.Fatalf("marshalJSONB() returned error: %v", err)
			}

			var got map[string]string
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("marshalJSONB() produced invalid JSON %s: %v", raw, err)
			}
			if got["title"] != tt.want {
				t.Errorf("marshalJSONB() = %q, want %q", got["title"], tt.want)
			}
		})
	}
}
