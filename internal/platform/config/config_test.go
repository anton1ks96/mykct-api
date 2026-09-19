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

// setRequiredEnv заполняет обязательные переменные, без которых разбор конфига
// не доходит до проверяемых настроек.
func setRequiredEnv(t *testing.T) {
	t.Helper()

	t.Setenv("MONGO_DATABASE", "mykct")
	t.Setenv("AUTH_JWT_SIGNING_KEY", "test-signing-key")
	t.Setenv("AUTH_TEST_MODE", "true")
	t.Setenv("SCHEDULE_PORTAL_URL", "https://portal.example")
	t.Setenv("ATTENDANCE_PORTAL_URL", "https://portal.example")
	t.Setenv("PERFORMANCE_PORTAL_URL", "https://portal.example")
}

// TestLeaderboardSecretRequired - включённый рейтинг без секрета должен ронять
// старт: без секрета псевдонимы сводятся к логинам перебором.
func TestLeaderboardSecretRequired(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ATTENDANCE_LEADERBOARD_ENABLED", "true")

	var cfg Config
	if err := setFromEnv(&cfg); err == nil {
		t.Error("setFromEnv() accepted enabled leaderboard without alias secret")
	}
}

// TestLeaderboardSecretTooShort - короткий секрет не проходит.
func TestLeaderboardSecretTooShort(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ATTENDANCE_LEADERBOARD_ENABLED", "true")
	t.Setenv("ATTENDANCE_LEADERBOARD_ALIAS_SECRET", "short")

	var cfg Config
	if err := setFromEnv(&cfg); err == nil {
		t.Error("setFromEnv() accepted an alias secret shorter than the minimum")
	}
}

// TestLeaderboardDisabledNeedsNoSecret - выключенному рейтингу секрет не нужен.
func TestLeaderboardDisabledNeedsNoSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ATTENDANCE_LEADERBOARD_ENABLED", "false")

	var cfg Config
	if err := setFromEnv(&cfg); err != nil {
		t.Fatalf("setFromEnv() returned error: %v", err)
	}
	if cfg.Attendance.Leaderboard.Enabled {
		t.Error("leaderboard is enabled, want disabled")
	}
}

// TestLeaderboardDefaults - дефолты разбираются и согласованы между собой.
func TestLeaderboardDefaults(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ATTENDANCE_LEADERBOARD_ENABLED", "true")
	t.Setenv("ATTENDANCE_LEADERBOARD_ALIAS_SECRET", "0123456789abcdef0123456789abcdef")

	var cfg Config
	if err := setFromEnv(&cfg); err != nil {
		t.Fatalf("setFromEnv() returned error: %v", err)
	}

	lb := cfg.Attendance.Leaderboard
	if lb.TopSize != 10 {
		t.Errorf("TopSize = %d, want 10", lb.TopSize)
	}
	if lb.MinParticipants != 10 {
		t.Errorf("MinParticipants = %d, want 10", lb.MinParticipants)
	}
	if lb.RefreshInterval != time.Hour {
		t.Errorf("RefreshInterval = %v, want 1h", lb.RefreshInterval)
	}
	if lb.RefreshTTL != 6*time.Hour {
		t.Errorf("RefreshTTL = %v, want 6h", lb.RefreshTTL)
	}
	if lb.StudentDelay != 2*time.Second {
		t.Errorf("StudentDelay = %v, want 2s", lb.StudentDelay)
	}
	if lb.BatchSize != 40 {
		t.Errorf("BatchSize = %d, want 40", lb.BatchSize)
	}
	if lb.EmptyRunsLimit != 5 {
		t.Errorf("EmptyRunsLimit = %d, want 5", lb.EmptyRunsLimit)
	}
}

// TestLeaderboardMinParticipantsFloor - порог ниже допустимого не проходит:
// в выборке из пары человек рейтинг перестаёт быть анонимным.
func TestLeaderboardMinParticipantsFloor(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ATTENDANCE_LEADERBOARD_ENABLED", "true")
	t.Setenv("ATTENDANCE_LEADERBOARD_ALIAS_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("ATTENDANCE_LEADERBOARD_MIN_PARTICIPANTS", "2")

	var cfg Config
	if err := setFromEnv(&cfg); err == nil {
		t.Error("setFromEnv() accepted a participants threshold below the floor")
	}
}
