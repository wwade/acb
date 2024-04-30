package portfolio

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tsiemens/acb/date"
	decimal_opt "github.com/tsiemens/acb/decimal_value"
)

func TestCalcSecurityCumulativeCapitalGains(t *testing.T) {
	getTx := func(sec, ymd string) *Tx {
		dateVal, err := date.Parse("2006-01-02", ymd)
		require.NoError(t, err)
		return &Tx{
			Security:       sec,
			SettlementDate: dateVal,
		}
	}
	type txInfo struct {
		sec         string
		ymd         string
		capGain     float64
		grossIncome float64
	}
	getDelta := func(spec txInfo) *TxDelta {
		return &TxDelta{
			Tx:          getTx(spec.sec, spec.ymd),
			CapitalGain: decimal_opt.NewFromFloat(spec.capGain),
			GrossIncome: decimal.NewFromFloat(spec.grossIncome),
		}
	}

	deltas := []*TxDelta{
		getDelta(txInfo{sec: "VXUS", ymd: "2015-01-13", capGain: 50, grossIncome: 10}),
		getDelta(txInfo{sec: "MRNA", ymd: "2015-01-13", capGain: 100, grossIncome: 20}),
		getDelta(txInfo{sec: "VXUS", ymd: "2015-02-12", capGain: 20, grossIncome: 5}),
		getDelta(txInfo{sec: "MRNA", ymd: "2016-03-25", capGain: 200, grossIncome: 30}),
		getDelta(txInfo{sec: "VXUS", ymd: "2016-03-26", capGain: 250, grossIncome: 40}),
		getDelta(txInfo{sec: "MRNA", ymd: "2016-03-27", capGain: 300, grossIncome: 50}),
	}

	cc := CalcSecurityCumulativeCapitalGains(deltas)

	assert.Len(t, cc.CapitalGainsYearTotals, 2)
	assert.Equal(t, decimal.NewFromFloat(100+50+20+200+250+300).String(), cc.CapitalGainsTotal.String())
	assert.Equal(t, decimal.NewFromFloat(100+50+20).String(), cc.CapitalGainsYearTotals[2015].String())
	assert.Equal(t, decimal.NewFromFloat(200+250+300).String(), cc.CapitalGainsYearTotals[2016].String())

	assert.Len(t, cc.GrossIncomeByYear, 2)
	assert.Equal(t, decimal.NewFromFloat(20+10+5+30+40+50).String(), cc.GrossIncomeTotal.String())
	assert.Equal(t, decimal.NewFromFloat(20+10+5).String(), cc.GrossIncomeByYear[2015].String())
	assert.Equal(t, decimal.NewFromFloat(30+40+50).String(), cc.GrossIncomeByYear[2016].String())
}

func TestCalcCumulativeCapitalGains(t *testing.T) {
	secGains := map[string]*CumulativeCapitalGains{
		"VXUS": {
			CapitalGainsTotal: decimal_opt.NewFromFloat(1000),
			CapitalGainsYearTotals: map[int]decimal_opt.DecimalOpt{
				2015: decimal_opt.NewFromFloat(750),
				2016: decimal_opt.NewFromFloat(250),
			},
			GrossIncomeTotal: decimal.NewFromFloat(2500),
			GrossIncomeByYear: map[int]decimal.Decimal{
				2015: decimal.NewFromFloat(1500),
				2016: decimal.NewFromFloat(1000),
			},
		},
		"VTI": {
			CapitalGainsTotal: decimal_opt.NewFromFloat(250),
			CapitalGainsYearTotals: map[int]decimal_opt.DecimalOpt{
				2015: decimal_opt.NewFromFloat(50),
				2016: decimal_opt.NewFromFloat(200),
			},
			GrossIncomeTotal: decimal.NewFromFloat(1000),
			GrossIncomeByYear: map[int]decimal.Decimal{
				2015: decimal.NewFromFloat(900),
				2016: decimal.NewFromFloat(100),
			},
		},
	}

	cc := CalcCumulativeCapitalGains(secGains)
	assert.Equal(t, decimal.NewFromFloat(1250).String(), cc.CapitalGainsTotal.String())
	assert.Equal(t, decimal.NewFromFloat(3500).String(), cc.GrossIncomeTotal.String())
	assert.Len(t, cc.CapitalGainsYearTotals, 2)
	assert.Equal(t, decimal.NewFromFloat(750+50).String(), cc.CapitalGainsYearTotals[2015].String())
	assert.Equal(t, decimal.NewFromFloat(250+200).String(), cc.CapitalGainsYearTotals[2016].String())
	assert.Len(t, cc.GrossIncomeByYear, 2)
	assert.Equal(t, decimal.NewFromFloat(1500+900).String(), cc.GrossIncomeByYear[2015].String())
	assert.Equal(t, decimal.NewFromFloat(1000+100).String(), cc.GrossIncomeByYear[2016].String())
}
