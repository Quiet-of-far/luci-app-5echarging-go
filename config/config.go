package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Room struct {
	Building   string   `json:"building"`
	Room       string   `json:"room"`
	Label      string   `json:"label"`
	Recipients []string `json:"recipients"`
}

type ScheduleConfig struct {
	IntervalMinutes int   `json:"interval_minutes"`
	CheckHours      []int `json:"check_hours"`
}

type EmailConfig struct {
	Enabled         bool     `json:"enabled"`
	AlertOncePerDay bool     `json:"alert_once_per_day"`
	SMTPHost        string   `json:"smtp_host"`
	SMTPPort        int      `json:"smtp_port"`
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	From            string   `json:"from"`
	To              []string `json:"to"`
}

type PredictionConfig struct {
	SampleCount            int     `json:"sample_count"`
	CustomDailyConsumption float64 `json:"custom_daily_consumption"`
	CustomDailyRateLegacy  float64 `json:"custom_daily_rate"`
}

type Config struct {
	Rooms              []Room           `json:"rooms"`
	Schedule           ScheduleConfig   `json:"schedule"`
	LowEnergyThreshold float64          `json:"low_energy_threshold"`
	ThresholdLegacy    float64          `json:"threshold"`
	DepletionAlertDays int              `json:"depletion_alert_days"`
	Email              EmailConfig      `json:"email"`
	Prediction         PredictionConfig `json:"prediction"`
	DBPath             string           `json:"db_path"`
	MaxRecordsPerRoom  int              `json:"max_records_per_room"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	var rawDefaults struct {
		MaxRecordsPerRoom *int `json:"max_records_per_room"`
	}
	if err := json.Unmarshal(data, &rawDefaults); err != nil {
		return nil, err
	}

	cfg.applyDefaults(rawDefaults.MaxRecordsPerRoom != nil)
	cfg.resolvePaths(path)
	return &cfg, nil
}

func (c *Config) applyDefaults(hasMaxRecordsPerRoom bool) {
	if c.LowEnergyThreshold == 0 {
		if c.ThresholdLegacy > 0 {
			c.LowEnergyThreshold = c.ThresholdLegacy
		} else {
			c.LowEnergyThreshold = 15.0
		}
	}
	if c.DBPath == "" {
		c.DBPath = "5echarging.bbolt"
	}
	if !hasMaxRecordsPerRoom {
		c.MaxRecordsPerRoom = 500
	}
	if c.Prediction.SampleCount == 0 {
		c.Prediction.SampleCount = 10
	}
	if c.Prediction.CustomDailyConsumption == 0 && c.Prediction.CustomDailyRateLegacy > 0 {
		c.Prediction.CustomDailyConsumption = c.Prediction.CustomDailyRateLegacy
	}
	if c.Schedule.IntervalMinutes == 0 && len(c.Schedule.CheckHours) == 0 {
		c.Schedule.IntervalMinutes = 60
	}
}

func (c *Config) resolvePaths(configPath string) {
	if filepath.IsAbs(c.DBPath) {
		return
	}

	baseDir := filepath.Dir(configPath)
	if baseDir == "" {
		baseDir = "."
	}
	c.DBPath = filepath.Clean(filepath.Join(baseDir, c.DBPath))
}
