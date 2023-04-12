package portfolio

import (
	"math"
	"sort"

	"github.com/tsiemens/acb/util"
)

type Stat struct {
	Total      float64
	YearTotals map[int]float64
}

type CumulativeCapitalGains struct {
	CapitalGainsTotal      float64
	CapitalGainsYearTotals map[int]float64
	GrossIncomeTotal       float64
	GrossIncomeByYear      map[int]float64
}

func (g *CumulativeCapitalGains) CapitalGainsYearTotalsKeysSorted() []int {
	years := util.IntFloat64MapKeys(g.CapitalGainsYearTotals)
	sort.Ints(years)
	return years
}

func CalcSecurityCumulativeCapitalGains(deltas []*TxDelta) *CumulativeCapitalGains {
	cc := &CumulativeCapitalGains{
		CapitalGainsYearTotals: map[int]float64{},
		GrossIncomeByYear:      map[int]float64{},
	}
	for _, d := range deltas {
		if !math.IsNaN(d.CapitalGain) {
			cc.CapitalGainsTotal += d.CapitalGain
			cc.CapitalGainsYearTotals[d.Tx.SettlementDate.Year()] += d.CapitalGain
			cc.GrossIncomeTotal += d.GrossIncome
			cc.GrossIncomeByYear[d.Tx.SettlementDate.Year()] += d.GrossIncome
		}
	}
	return cc
}

func CalcCumulativeCapitalGains(secGains map[string]*CumulativeCapitalGains) *CumulativeCapitalGains {
	cc := &CumulativeCapitalGains{
		CapitalGainsYearTotals: map[int]float64{},
		GrossIncomeByYear:      map[int]float64{},
	}
	for _, gains := range secGains {
		cc.CapitalGainsTotal += gains.CapitalGainsTotal
		cc.GrossIncomeTotal += gains.GrossIncomeTotal
		for year, yearGains := range gains.CapitalGainsYearTotals {
			cc.CapitalGainsYearTotals[year] += yearGains
		}
		for year, gross := range gains.GrossIncomeByYear {
			cc.GrossIncomeByYear[year] += gross
		}
	}
	return cc
}
