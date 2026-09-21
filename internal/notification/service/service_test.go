package service

import (
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/pkg/collegetime"
)

func TestApnsConfigSoundOutsideQuietHours(t *testing.T) {
	cases := []struct {
		name  string
		hour  int
		sound bool
	}{
		{name: "утро", hour: 9, sound: true},
		{name: "день", hour: 15, sound: true},
		{name: "вечер", hour: 21, sound: true},
		{name: "начало тихих часов", hour: 22, sound: false},
		{name: "ночь", hour: 3, sound: false},
		{name: "конец тихих часов", hour: 7, sound: false},
		{name: "утро после тихих часов", hour: 8, sound: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 9, 21, tc.hour, 30, 0, 0, collegetime.TZ())

			cfg := apnsConfig(now)
			if !tc.sound {
				if cfg != nil {
					t.Fatalf("expected no apns config at %d:30, got %+v", tc.hour, cfg)
				}
				return
			}

			if cfg == nil || cfg.Payload == nil || cfg.Payload.Aps == nil {
				t.Fatalf("expected apns config at %d:30, got %+v", tc.hour, cfg)
			}
			if cfg.Payload.Aps.Sound != "default" {
				t.Fatalf("expected default sound at %d:30, got %q", tc.hour, cfg.Payload.Aps.Sound)
			}
		})
	}
}
