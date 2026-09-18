package config

import (
	"testing"
	"time"
)

// TestParseWeekdays - дни активного опроса читаются как есть, а опечатка в
// .env должна ронять старт, а не молча оставлять воркер на холостом интервале.
func TestParseWeekdays(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []time.Weekday
	}{
		{"значение по умолчанию", "Fri,Sat,Sun", []time.Weekday{time.Friday, time.Saturday, time.Sunday}},
		{"регистр и пробелы", " fri , SAT ", []time.Weekday{time.Friday, time.Saturday}},
		{"пустая строка", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, err := parseWeekdays(tt.raw)
			if err != nil {
				t.Fatalf("parseWeekdays(%q) returned error: %v", tt.raw, err)
			}
			if len(days) != len(tt.want) {
				t.Fatalf("parseWeekdays(%q) = %v, want %v days", tt.raw, days, len(tt.want))
			}
			for _, day := range tt.want {
				if !days[day] {
					t.Errorf("parseWeekdays(%q) misses %v", tt.raw, day)
				}
			}
		})
	}
}

// TestParseWeekdaysRejectsUnknown - неизвестный день не проходит молча.
func TestParseWeekdaysRejectsUnknown(t *testing.T) {
	if _, err := parseWeekdays("Fri,Freitag"); err == nil {
		t.Error("parseWeekdays() accepted an unknown weekday")
	}
}
