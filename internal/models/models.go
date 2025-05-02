package models

import "time"

type Data struct {
	PriceHistory map[string]map[time.Time]float64
	ActualPrices map[string]float64
}

type Frequency string

const (
	Monthly Frequency = "monthly"
	Weekly  Frequency = "weekly"
	Daily   Frequency = "daily"
)

type Period struct {
	Frequency Frequency
	Hour      uint
	Minute    uint
}
