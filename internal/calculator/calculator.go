package calculator

import (
	"fmt"
	"strings"
	"time"

	"github.com/mayye4ka/cryptocalc/internal/models"
)

const bankInterestRate = 5

type Calculator struct {
	investSize   uint
	investPeriod models.Period
	currencies   []string
	dateFrom     time.Time
	dateTo       time.Time
}

func New(investSize uint, investPeriod models.Period, currencies []string, dateFrom, dateTo time.Time) *Calculator {
	return &Calculator{
		investSize:   investSize,
		investPeriod: investPeriod,
		currencies:   currencies,
		dateFrom:     dateFrom,
		dateTo:       dateTo,
	}
}

var reportTemplate = `Actual liquidation price is %.2f$.
If you were just collecting cash you'd got %d$.
If you were depositing bank with 5%% rate you'd got %.2f$.
Your actual token balance is:
%s`

var tokenBalanceTemplate = `* %s: %.2f priced %.2f$`

func (c *Calculator) CalculateProfitsReport(data *models.Data) (string, error) {
	dayDiff, monthDiff := 0, 0
	if c.investPeriod.Frequency == models.Monthly {
		monthDiff = 1
	} else if c.investPeriod.Frequency == models.Weekly {
		dayDiff = 7
	} else {
		dayDiff = 1
	}
	depositStarts := []time.Time{}
	tokensBalance := map[string]float64{}
	totalInvested := uint(0)
	startTime := time.Date(c.dateFrom.Year(), c.dateFrom.Month(), c.dateFrom.Day(), int(c.investPeriod.Hour), int(c.investPeriod.Minute), 0, 0, time.Local)
	for startTime.Before(c.dateTo) {
		depositStarts = append(depositStarts, startTime)
		for _, token := range c.currencies {
			tokensBalance[token] += data.PriceHistory[token][startTime] * float64(c.investSize)
			totalInvested += c.investSize
		}
		startTime = time.Date(
			startTime.Year(), startTime.Month()+time.Month(monthDiff),
			startTime.Day()+dayDiff, startTime.Hour(), startTime.Minute(), 0, 0, startTime.Location(),
		)
	}

	var liquidationPriceUsd float64 = 0
	for token, amount := range tokensBalance {
		liquidationPriceUsd += amount / data.ActualPrices[token]
	}

	var totalBankDeposit float64 = 0
	for _, depositStart := range depositStarts {
		totalBankDeposit += calculateDepositProfit(depositStart, time.Now(), float64(int(c.investSize)*len(c.currencies)))
	}
	tokenReports := []string{}
	for token, amount := range tokensBalance {
		tokenReports = append(tokenReports, fmt.Sprintf(tokenBalanceTemplate, token, amount, amount/data.ActualPrices[token]))
	}
	tokensReport := strings.Join(tokenReports, "\n")
	report := fmt.Sprintf(reportTemplate, liquidationPriceUsd, totalInvested, totalBankDeposit, tokensReport)

	return report, nil
}

func calculateDepositProfit(from, to time.Time, value float64) float64 {
	daysDiff := to.Sub(from).Hours() / 24
	var profit float64 = value
	for daysDiff/365 > 0 {
		profit += profit * bankInterestRate / 100
		daysDiff -= 365
	}
	profit += ((profit * bankInterestRate / 100) * daysDiff) / 365
	return profit
}
