package collector

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mayye4ka/cryptocalc/internal/models"
	"golang.org/x/sync/errgroup"
)

const (
	dataFile           = "data.json"
	binanceHistoryUrl  = "https://data.binance.vision/data/spot/daily/klines/%sUSDT/5m/%sUSDT-5m-%s.zip"
	timeFormat         = "2006-01-02"
	maxCollectRoutines = 256
)

type DataCollector struct {
	forceCollect     bool
	dateFrom, dateTo time.Time
	currencies       []string
	period           models.Period
}

func New(forceCollect bool, dateFrom, dateTo time.Time, currencies []string, period models.Period) *DataCollector {
	return &DataCollector{
		forceCollect: forceCollect,
		dateFrom:     dateFrom,
		dateTo:       dateTo,
		currencies:   currencies,
		period:       period,
	}
}

func (dc *DataCollector) CollectData(ctx context.Context) (*models.Data, error) {
	df, err := os.Open(dataFile)
	if err == nil && !dc.forceCollect {
		var data models.Data
		err := json.NewDecoder(df).Decode(&data)
		if err != nil {
			return nil, err
		}
		return &data, nil
	}
	data, err := dc.collectData(ctx)
	if err != nil {
		return nil, err
	}
	df, err = os.Create(dataFile)
	if err != nil {
		return nil, err
	}
	err = json.NewEncoder(df).Encode(data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (dc *DataCollector) collectData(ctx context.Context) (*models.Data, error) {
	data := models.Data{
		PriceHistory: map[string]map[time.Time]float64{},
		ActualPrices: map[string]float64{},
	}
	dataMu := sync.Mutex{}
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(maxCollectRoutines)

	dayDiff, monthDiff := 0, 0
	if dc.period.Frequency == models.Monthly {
		monthDiff = 1
	} else if dc.period.Frequency == models.Weekly {
		dayDiff = 7
	} else {
		dayDiff = 1
	}

	for _, token := range dc.currencies {
		startTime := time.Date(dc.dateFrom.Year(), dc.dateFrom.Month(), dc.dateFrom.Day(), int(dc.period.Hour), int(dc.period.Minute), 0, 0, time.Local)
		for startTime.Before(dc.dateTo) {
			token := token
			collectTime := startTime
			eg.Go(func() error {
				price, err := getPrice(ctx, token, collectTime)
				if err != nil {
					return err
				}
				dataMu.Lock()
				if data.PriceHistory[token] == nil {
					data.PriceHistory[token] = map[time.Time]float64{}
				}
				data.PriceHistory[token][collectTime] = price

				dataMu.Unlock()
				return nil
			})
			startTime = time.Date(
				startTime.Year(), startTime.Month()+time.Month(monthDiff), startTime.Day()+dayDiff, startTime.Hour(), startTime.Minute(), 0, 0, startTime.Location(),
			)
		}
	}

	for _, token := range dc.currencies {
		price, err := getActualPrice(ctx, token)
		if err != nil {
			log.Fatal(err)
		}
		data.ActualPrices[token] = price
	}

	err := eg.Wait()
	if err != nil {
		return nil, err
	}

	return &data, nil
}

func parseTs(ts string) (time.Time, error) {
	ts = ts[:10]
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}

func getPrice(_ context.Context, token string, t time.Time) (float64, error) {
	token = strings.ToUpper(token)
	resp, err := http.Get(fmt.Sprintf(binanceHistoryUrl, token, token, t.Format(timeFormat)))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	archBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	zipReader, err := zip.NewReader(bytes.NewReader(archBody), int64(len(archBody)))
	if err != nil {
		return 0, err
	}
	csvFile, err := zipReader.File[0].Open()
	if err != nil {
		return 0, err
	}
	defer csvFile.Close()

	csvRecords, err := csv.NewReader(csvFile).ReadAll()
	if err != nil {
		return 0, err
	}

	for _, row := range csvRecords {
		openTime, err := parseTs(row[0])
		if err != nil {
			return 0, err
		}
		closeTime, err := parseTs(row[6])
		if err != nil {
			return 0, err
		}
		open, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			return 0, err
		}
		close, err := strconv.ParseFloat(row[4], 64)
		if err != nil {
			return 0, err
		}
		if (t.After(openTime) && t.Before(closeTime)) || t.Equal(openTime) || t.Equal(closeTime) {
			return 1 / ((open + close) / 2), nil
		}
	}
	return 0, fmt.Errorf("time %s not found", t.Format(time.RFC3339))
}

func getActualPrice(ctx context.Context, token string) (float64, error) {
	return getPrice(ctx, token, time.Now().Add(-time.Hour*24*2))
}
