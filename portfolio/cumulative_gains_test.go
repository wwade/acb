package portfolio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tsiemens/acb/date"
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
			CapitalGain: capGain,
			GrossIncome: gross,
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
	assert.Equal(t, &CumulativeCapitalGains{
		CapitalGainsTotal: 100 + 50 + 20 + 200 + 250 + 300,
		CapitalGainsYearTotals: map[int]float64{
			2015: 100 + 50 + 20,
			2016: 200 + 250 + 300,
		},
		GrossIncomeTotal: 20 + 10 + 5 + 30 + 40 + 50,
		GrossIncomeByYear: map[int]float64{
			2015: 20 + 10 + 5,
			2016: 30 + 40 + 50,
		},
	}, cc)
}

func TestCalcCumulativeCapitalGains(t *testing.T) {
	secGains := map[string]*CumulativeCapitalGains{
		"VXUS": {
			CapitalGainsTotal: 1000,
			CapitalGainsYearTotals: map[int]float64{
				2015: 750,
				2016: 250,
			},
			GrossIncomeTotal: 2500,
			GrossIncomeByYear: map[int]float64{
				2015: 1500,
				2016: 1000,
			},
		},
		"VTI": {
			CapitalGainsTotal: 250,
			CapitalGainsYearTotals: map[int]float64{
				2015: 50,
				2016: 200,
			},
			GrossIncomeTotal: 1000,
			GrossIncomeByYear: map[int]float64{
				2015: 900,
				2016: 100,
			},
		},
	}

	cc := CalcCumulativeCapitalGains(secGains)
	assert.Equal(t, float64(1250), cc.CapitalGainsTotal)
	assert.Equal(t, float64(3500), cc.GrossIncomeTotal)
	assert.Equal(t, map[int]float64{2015: 750 + 50, 2016: 250 + 200}, cc.CapitalGainsYearTotals)
	assert.Equal(t, map[int]float64{2015: 1500 + 900, 2016: 1000 + 100}, cc.GrossIncomeByYear)
}
