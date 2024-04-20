package portfolio

import (
	"sort"

	"github.com/shopspring/decimal"
	decimal_opt "github.com/tsiemens/acb/decimal_value"
	"github.com/tsiemens/acb/util"
)

type Stat struct {
	Total      decimal_opt.DecimalOpt
	YearTotals map[int]decimal_opt.DecimalOpt
}

type CumulativeCapitalGains struct {
	CapitalGainsTotal      decimal_opt.DecimalOpt
	CapitalGainsYearTotals map[int]decimal_opt.DecimalOpt
	GrossIncomeTotal       decimal.Decimal
	GrossIncomeByYear      map[int]decimal.Decimal
}

func (g *CumulativeCapitalGains) CapitalGainsYearTotalsKeysSorted() []int {
	years := util.IntDecimalOptMapKeys(g.CapitalGainsYearTotals)
	sort.Ints(years)
	return years
}

func CalcSecurityCumulativeCapitalGains(deltas []*TxDelta) *CumulativeCapitalGains {
	var capGainsTotal decimal_opt.DecimalOpt
	capGainsYearTotals := util.NewDefaultMap[int, decimal_opt.DecimalOpt](
		func(_ int) decimal_opt.DecimalOpt { return decimal_opt.Zero })
	var grossIncomeTotal decimal.Decimal
	grossIncomeYearTotals := util.NewDefaultMap[int, decimal.Decimal](
		func(_ int) decimal.Decimal { return decimal.Zero })

	for _, d := range deltas {
		if !d.CapitalGain.IsNull {
			capGainsTotal = capGainsTotal.Add(d.CapitalGain)
			yearTotalSoFar := capGainsYearTotals.Get(d.Tx.SettlementDate.Year())
			capGainsYearTotals.Set(d.Tx.SettlementDate.Year(), yearTotalSoFar.Add(d.CapitalGain))
		}
		if !d.CapitalGain.IsNull || !d.GrossIncome.IsZero() {
			grossIncomeTotal = grossIncomeTotal.Add(d.GrossIncome)
			yearTotalSoFar := grossIncomeYearTotals.Get(d.Tx.SettlementDate.Year())
			grossIncomeYearTotals.Set(d.Tx.SettlementDate.Year(), yearTotalSoFar.Add(d.GrossIncome))
		}
	}
	return &CumulativeCapitalGains{
		CapitalGainsTotal:      capGainsTotal,
		CapitalGainsYearTotals: capGainsYearTotals.EjectMap(),
		GrossIncomeTotal:       grossIncomeTotal,
		GrossIncomeByYear:      grossIncomeYearTotals.EjectMap(),
	}
}

func CalcCumulativeCapitalGains(secGains map[string]*CumulativeCapitalGains) *CumulativeCapitalGains {
	var capGainsTotal decimal_opt.DecimalOpt
	capGainsYearTotals := util.NewDefaultMap[int, decimal_opt.DecimalOpt](
		func(_ int) decimal_opt.DecimalOpt { return decimal_opt.Zero })
	var grossIncomeTotal decimal.Decimal
	grossIncomeYearTotals := util.NewDefaultMap[int, decimal.Decimal](
		func(_ int) decimal.Decimal { return decimal.Zero })

	for _, gains := range secGains {
		capGainsTotal = capGainsTotal.Add(gains.CapitalGainsTotal)
		grossIncomeTotal = grossIncomeTotal.Add(gains.GrossIncomeTotal)
		for year, yearGains := range gains.CapitalGainsYearTotals {
			yearTotalSoFar := capGainsYearTotals.Get(year)
			capGainsYearTotals.Set(year, yearTotalSoFar.Add(yearGains))
		}
		for year, gross := range gains.GrossIncomeByYear {
			yearTotalSoFar := grossIncomeYearTotals.Get(year)
			grossIncomeYearTotals.Set(year, yearTotalSoFar.Add(gross))
		}
	}

	return &CumulativeCapitalGains{
		CapitalGainsTotal:      capGainsTotal,
		CapitalGainsYearTotals: capGainsYearTotals.EjectMap(),
		GrossIncomeTotal:       grossIncomeTotal,
		GrossIncomeByYear:      grossIncomeYearTotals.EjectMap(),
	}
}
