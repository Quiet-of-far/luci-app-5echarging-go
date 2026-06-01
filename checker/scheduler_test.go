package checker

import (
	"path/filepath"
	"testing"
	"time"

	"luci-app-5echarging-go/config"
)

func TestAlertOncePerDayDisabledDoesNotThrottle(t *testing.T) {
	c := New(&config.Config{
		DBPath: filepath.Join(t.TempDir(), "5echarging.bbolt"),
		Email:  config.EmailConfig{AlertOncePerDay: false},
	}, nil)
	room := config.Room{Building: "44", Room: "1207"}
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.Local)

	if !c.shouldSendAlert(room, []string{alertLowEnergy}, now) {
		t.Fatal("first alert should be sent")
	}
	c.markAlertsSent(room, []string{alertLowEnergy}, now)
	if !c.shouldSendAlert(room, []string{alertLowEnergy}, now.Add(time.Hour)) {
		t.Fatal("alerts should not be throttled when alert_once_per_day is disabled")
	}
}

func TestAlertOncePerDayThrottlesByRoomAndLocalDate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "5echarging.bbolt")
	c := New(&config.Config{
		DBPath: dbPath,
		Email:  config.EmailConfig{AlertOncePerDay: true},
	}, nil)
	room := config.Room{Building: "44", Room: "1207"}
	otherRoom := config.Room{Building: "44", Room: "1208"}
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.Local)

	if !c.shouldSendAlert(room, []string{alertLowEnergy}, now) {
		t.Fatal("first alert of the day should be sent")
	}
	c.markAlertsSent(room, []string{alertLowEnergy}, now)
	if c.shouldSendAlert(room, []string{alertDepletion}, now.Add(2*time.Hour)) {
		t.Fatal("same room should be throttled after one alert on the same day")
	}
	restarted := New(&config.Config{
		DBPath: dbPath,
		Email:  config.EmailConfig{AlertOncePerDay: true},
	}, nil)
	if restarted.shouldSendAlert(room, []string{alertLowEnergy}, now.Add(2*time.Hour)) {
		t.Fatal("persisted alert state should throttle after restart")
	}
	if !c.shouldSendAlert(otherRoom, []string{alertLowEnergy}, now.Add(2*time.Hour)) {
		t.Fatal("different room should not be throttled")
	}
	if !c.shouldSendAlert(room, []string{alertLowEnergy}, now.Add(24*time.Hour)) {
		t.Fatal("same room should be allowed again on the next day")
	}
}
