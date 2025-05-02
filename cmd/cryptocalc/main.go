package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	flags "github.com/jessevdk/go-flags"
	"github.com/mayye4ka/cryptocalc/internal/calculator"
	"github.com/mayye4ka/cryptocalc/internal/collector"
	"github.com/mayye4ka/cryptocalc/internal/models"
)

const timeFormat = "2006-01-02"

type CmdFlags struct {
	ForceCollect  bool   `short:"f" long:"force-collect" description:"Do not use cached prices"`
	StartDate     string `short:"s" long:"start-date" description:"Investing start date, time format YYYY-MM-DD" default:"2024-06-25"`
	EndDate       string `short:"e" long:"end-date" description:"Investing end date, time format YYYY-MM-DD or now" default:"now"`
	Currencies    string `short:"c" long:"currencies" description:"Comma separated list of tokens" default:"btc,eth"`
	InvestSizeUsd uint   `short:"i" long:"invest-size" description:"Usd amount of investment each period into each token" default:"50"`
	InvestPeriod  string `short:"p" long:"invest-period" description:"Expression describing invest period since starting date" default:"monthly" choice:"monthly" choice:"weekly" choice:"daily"`
	InvestTime    string `short:"t" long:"invest-time" description:"HH:MM time of the day when to invest" default:"19:00"`
	OutputFile    string `short:"o" long:"output-file" description:"Output file name" default:""`
}

type Config struct {
	ForceCollect bool
	From         time.Time
	To           time.Time
	Currencies   []string
	InvestSize   uint
	InvestPeriod models.Period
	OutputFile   string
}

func getConfig() (Config, error) {
	var cmdFlags CmdFlags
	_, err := flags.Parse(&cmdFlags)
	if err != nil && !errors.Is(err, flags.ErrHelp) {
		if w, ok := err.(*flags.Error); ok {
			if w.Type == flags.ErrHelp {
				os.Exit(0)
			}
		}
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	currencies := strings.Split(strings.ToLower(cmdFlags.Currencies), ",")
	startDate, err := time.Parse(timeFormat, cmdFlags.StartDate)
	if err != nil {
		return Config{}, fmt.Errorf("parse start date: %w", err)
	}
	var endDate time.Time
	if cmdFlags.EndDate == "now" {
		endDate = time.Now()
	} else {
		endDate, err = time.Parse(timeFormat, cmdFlags.EndDate)
		if err != nil {
			return Config{}, fmt.Errorf("parse end date: %w", err)
		}
	}

	investTime := strings.Split(cmdFlags.InvestTime, ":")
	if len(investTime) != 2 {
		return Config{}, fmt.Errorf("bad invest time")
	}
	hour, err := strconv.ParseUint(investTime[0], 10, 64)
	if len(investTime) != 2 {
		return Config{}, fmt.Errorf("parse invest hour: %w", err)
	}
	minute, err := strconv.ParseUint(investTime[1], 10, 64)
	if len(investTime) != 2 {
		return Config{}, fmt.Errorf("parse invest minute: %w", err)
	}
	return Config{
		ForceCollect: cmdFlags.ForceCollect,
		From:         startDate,
		To:           endDate,
		Currencies:   currencies,
		InvestSize:   cmdFlags.InvestSizeUsd,
		OutputFile:   cmdFlags.OutputFile,
		InvestPeriod: models.Period{
			Frequency: models.Frequency(cmdFlags.InvestPeriod),
			Hour:      uint(hour),
			Minute:    uint(minute),
		},
	}, nil
}

func getData(ctx context.Context, cfg Config) (*models.Data, error) {
	return collector.New(
		cfg.ForceCollect,
		cfg.From,
		cfg.To,
		cfg.Currencies,
		cfg.InvestPeriod,
	).CollectData(ctx)
}

func getReport(data *models.Data, cfg Config) (string, error) {
	return calculator.New(
		cfg.InvestSize, cfg.InvestPeriod, cfg.Currencies, cfg.From, cfg.To,
	).CalculateProfitsReport(data)
}

func main() {
	ctx, cf := context.WithCancel(context.Background())
	defer cf()
	termChan := make(chan os.Signal, 1)
	signal.Notify(termChan, syscall.SIGTERM)
	go func() {
		<-termChan
		cf()
	}()
	cfg, err := getConfig()
	if err != nil {
		log.Fatal(err)
	}
	data, err := getData(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	report, err := getReport(data, cfg)
	if err != nil {
		log.Fatal(err)
	}
	output := os.Stdout
	if cfg.OutputFile != "" {
		of, err := os.Create(cfg.OutputFile)
		if err != nil {
			log.Fatal(err)
		}
		defer of.Close()
		output = of
	}
	fmt.Fprintf(output, "%s\n", report)
}
