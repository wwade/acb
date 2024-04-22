package portfolio

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/tsiemens/acb/date"
	decimal_opt "github.com/tsiemens/acb/decimal_value"
)

func TestCalcSecurityCumulativeCapitalGains(t *testing.T) {
	var day uint32
	getTx := func(year uint32) *Tx {
		day += 1
		return &Tx{
			Security:       "VXUS",
			SettlementDate: date.New(year, 1, day),
		}
	}
	getDelta := func(year uint32, capGain, gross float64) *TxDelta {
		return &TxDelta{
			Tx:          getTx(year),
			CapitalGain: decimal_opt.NewFromFloat(capGain),
			GrossIncome: decimal.NewFromFloat(gross),
		}
	}

	deltas := []*TxDelta{
		getDelta(2015, 100, 20),
		getDelta(2015, 50, 10),
		getDelta(2015, 20, 5),
		getDelta(2016, 200, 30),
		getDelta(2016, 250, 40),
		getDelta(2016, 300, 50),
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
