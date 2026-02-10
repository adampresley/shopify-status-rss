package main

import (
	"github.com/adampresley/configinator"
	"github.com/adampresley/mux"
)

type Config struct {
	mux.Config
	CronSchedule  string `flag:"cronschedule" env:"CRON_SCHEDULE" default:"*/30 * * * *" description:"cron schedule for status updates"`
	DSN           string `flag:"dsn" env:"DSN" default:"file:./shopify-status-rss.db" description:"database connection string"`
	LogLevel      string `flag:"loglevel" env:"LOG_LEVEL" default:"info" description:"slog log leve. defaults to info"`
	StatusPageURL string `flag:"statuspageurl" env:"STATUS_PAGE_URL" default:"https://my.shopifystatus.com" description:"status page URL"`
}

func LoadConfig() *Config {
	result := &Config{}
	configinator.Behold(result)
	return result
}
