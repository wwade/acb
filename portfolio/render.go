package portfolio

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/tsiemens/acb/date"
	decimal_opt "github.com/tsiemens/acb/decimal_value"
	"github.com/tsiemens/acb/util"
)

type _PrintHelper struct {
	PrintAllDecimals bool
}

var displayNanEnvSetting util.Optional[string]

func NaNString() string {
	if !displayNanEnvSetting.Present() {
		displayNanEnvSetting.Set(os.Getenv("DISPLAY_NAN"))
	}
	if displayNanEnvSetting.MustGet() == "" || displayNanEnvSetting.MustGet() == "0" {
		return "-"
	}
	return "NaN"
}

func humanizeDecimalStr(val string) string {
	if os.Getenv("HUMANIZE") == "" {
		return val
	}
	negative := ""
	if strings.HasPrefix(val, "-") {
		negative, val = val[:1], val[1:]
	}
	before, after, found := strings.Cut(val, ".")
	suffix := ""
	if found {
		suffix = fmt.Sprintf(".%s", after)
	}
	i, err := strconv.ParseInt(before, 10, 64)
	if err != nil {
		panic(err)
	}
	p := message.NewPrinter(language.English)
	return p.Sprintf("%s%d%s", negative, i, suffix)
}

func (h _PrintHelper) CurrStr(val decimal.Decimal) string {
	if h.PrintAllDecimals {
		return val.String()
	}
	return humanizeDecimalStr(val.StringFixed(2))
}

func (h _PrintHelper) OptCurrStr(val decimal_opt.DecimalOpt) string {
	if val.IsNull {
		return val.String()
	}
	return h.CurrStr(val.Decimal)
}

func (h _PrintHelper) DollarStr(val decimal_opt.DecimalOpt) string {
	if val.IsNull {
		return NaNString()
	}
	return "$" + h.OptCurrStr(val)
}

func (h _PrintHelper) CurrWithFxStr(val decimal.Decimal, curr Currency, rateToLocal decimal.Decimal) string {
	if curr == DEFAULT_CURRENCY {
		return h.DollarStr(decimal_opt.New(val))
	}
	return fmt.Sprintf("%s\n(%s %s)", h.DollarStr(decimal_opt.New(val.Mul(rateToLocal))), h.CurrStr(val), curr)
}

func strOrDash(useStr bool, str string) string {
	if useStr {
		return str
	}
	return "-"
}

func (h _PrintHelper) PlusMinusDollar(val decimal_opt.DecimalOpt, showPlus bool) string {
	if val.IsNull {
		return NaNString()
	}
	if val.IsNegative() {
		return fmt.Sprintf("-$%s", h.OptCurrStr(val.Neg()))
	}
	plus := ""
	if showPlus {
		plus = "+"
	}
	return fmt.Sprintf("%s$%s", plus, h.OptCurrStr(val))
}

type RenderTable struct {
	Header []string
	Rows   [][]string
	Footer []string
	Notes  []string
	Errors []error
}

type CostsTables struct {
	Total  *RenderTable
	Yearly *RenderTable
}

func RenderTxTableModel(
	deltas []*TxDelta, gains *CumulativeCapitalGains, renderFullDollarValues bool) *RenderTable {
	table := &RenderTable{}
	table.Header = []string{"Security", "Trade Date", "Settl. Date", "TX", "Amount", "Shares", "Amt/Share", "ACB",
		"Commission", "Cap. Gain", "Share Balance", "ACB +/-", "New ACB", "New ACB/Share",
		"Affiliate", "Memo",
	}

	ph := _PrintHelper{PrintAllDecimals: renderFullDollarValues}

	sawSuperficialLoss := false
	sawOverAppliedSfl := false

	for _, d := range deltas {
		superficialLossAsterix := ""
		specifiedSflIsForced := d.Tx.SpecifiedSuperficialLoss.Present() &&
			d.Tx.SpecifiedSuperficialLoss.MustGet().Force
		if d.IsSuperficialLoss() {
			extraSflNoteStr := ""
			if d.PotentiallyOverAppliedSfl {
				extraSflNoteStr = " [1]"
			}

			superficialLossAsterix = fmt.Sprintf(
				" *\n(SfL %s%s; %s/%s%s)",
				ph.PlusMinusDollar(d.SuperficialLoss, false),
				util.Tern(specifiedSflIsForced, "!", ""),
				d.SuperficialLossRatio.Numerator,
				d.SuperficialLossRatio.Denominator,
				extraSflNoteStr,
			)
			sawSuperficialLoss = true
			sawOverAppliedSfl = sawOverAppliedSfl || d.PotentiallyOverAppliedSfl
		}
		tx := d.Tx

		var preAcbPerShare decimal_opt.DecimalOpt
		if tx.Action == SELL && d.PreStatus.ShareBalance.IsPositive() {
			preAcbPerShare = d.PreStatus.TotalAcb.DivD(d.PreStatus.ShareBalance)
		}

		var affiliateName string
		if tx.Affiliate != nil {
			affiliateName = tx.Affiliate.Name()
		} else {
			affiliateName = GlobalAffiliateDedupTable.GetDefaultAffiliate().Name()
		}

		row := []string{d.Tx.Security, tx.TradeDate.String(), tx.SettlementDate.String(), tx.Action.String(),
			// Amount
			ph.CurrWithFxStr(tx.Shares.Mul(tx.AmountPerShare), tx.TxCurrency, tx.TxCurrToLocalExchangeRate),
			tx.Shares.String(),
			ph.CurrWithFxStr(tx.AmountPerShare, tx.TxCurrency, tx.TxCurrToLocalExchangeRate),
			// ACB of sale
			strOrDash(tx.Action == SELL, ph.DollarStr(preAcbPerShare.MulD(tx.Shares))),
			// Commission
			strOrDash(!tx.Commission.IsZero(),
				ph.CurrWithFxStr(tx.Commission, tx.CommissionCurrency, tx.CommissionCurrToLocalExchangeRate)),
			// Cap gains
			strOrDash(tx.Action == SELL, ph.PlusMinusDollar(d.CapitalGain, false)+superficialLossAsterix),
			util.Tern(d.PostStatus.ShareBalance.Equal(d.PostStatus.AllAffiliatesShareBalance),
				d.PostStatus.ShareBalance.String(),
				fmt.Sprintf("%s / %s", d.PostStatus.ShareBalance, d.PostStatus.AllAffiliatesShareBalance)),
			ph.PlusMinusDollar(d.AcbDelta(), true),
			ph.DollarStr(d.PostStatus.TotalAcb),
			// Acb per share
			strOrDash(d.PostStatus.ShareBalance.IsPositive(),
				ph.DollarStr(d.PostStatus.TotalAcb.DivD(d.PostStatus.ShareBalance))),
			affiliateName,
			tx.Memo,
		}
		table.Rows = append(table.Rows, row)
	}

	// Footer
	years := gains.CapitalGainsYearTotalsKeysSorted()
	yearStrs := []string{}
	yearValsStrs := []string{}
	for _, year := range years {
		yearStrs = append(yearStrs, fmt.Sprintf("%d", year))
		yearlyTotal := gains.CapitalGainsYearTotals[year]
		yearValsStrs = append(yearValsStrs, ph.PlusMinusDollar(yearlyTotal, false))
	}
	totalFooterLabel := "Total"
	totalFooterValsStr := ph.PlusMinusDollar(gains.CapitalGainsTotal, false)
	if len(years) > 0 {
		totalFooterLabel += "\n" + strings.Join(yearStrs, "\n")
		totalFooterValsStr += "\n" + strings.Join(yearValsStrs, "\n")
	}

	table.Footer = []string{"", "", "", "", "", "", "", "",
		totalFooterLabel, totalFooterValsStr, "", "", "", "", "", ""}

	// Notes
	if sawSuperficialLoss {
		table.Notes = append(table.Notes, " SfL = Superficial loss adjustment")
	}
	if sawOverAppliedSfl {
		table.Notes = append(table.Notes,
			" [1] Superficial loss was potentially over-applied, resulting in a lower-than-expected allowable capital loss.\n"+
				"     See I.1 vs I.2 under \"Interpretations of ACB distribution\" at https://github.com/tsiemens/acb/wiki/Superficial-Losses")
	}

	return table
}

// RenderAggregateCapitalGains generates a RenderTable that will render out to this:
//
//	| Year             | Capital Gains |
//	+------------------+---------------+
//	| 2000             | xxxx.xx       |
//	| 2001             | xxxx.xx       |
//	| Since inception  | xxxx.xx       |
func RenderAggregateCapitalGains(
	gains *CumulativeCapitalGains, renderFullDollarValues bool) *RenderTable {

	table := &RenderTable{}
	table.Header = []string{"Year", "Capital Gains"}

	ph := _PrintHelper{PrintAllDecimals: renderFullDollarValues}

	years := gains.CapitalGainsYearTotalsKeysSorted()
	for _, year := range years {
		yearlyTotal := gains.CapitalGainsYearTotals[year]
		table.Rows = append(
			table.Rows,
			[]string{fmt.Sprintf("%d", year), ph.PlusMinusDollar(yearlyTotal, false)})
	}
	table.Rows = append(
		table.Rows,
		[]string{"Since inception", ph.PlusMinusDollar(gains.CapitalGainsTotal, false)})

	return table
}

func RenderTotalCosts(allDeltas []*TxDelta, renderFullDollarValues bool) *CostsTables {
	ph := _PrintHelper{PrintAllDecimals: renderFullDollarValues}

	curCost := map[string]decimal_opt.DecimalOpt{}
	type costinfo struct {
		date    date.Date
		total   decimal_opt.DecimalOpt
		secCost map[string]decimal_opt.DecimalOpt
	}
	var costs []costinfo
	sort.Slice(allDeltas, func(i, j int) bool {
		return allDeltas[i].Tx.SettlementDate.Before(allDeltas[j].Tx.SettlementDate)
	})
	for _, d := range allDeltas {
		curCost[d.PostStatus.Security] = d.PostStatus.TotalAcb
		inf := costinfo{
			date:    d.Tx.SettlementDate,
			total:   decimal_opt.Zero,
			secCost: map[string]decimal_opt.DecimalOpt{},
		}
		for s, v := range curCost {
			inf.total = inf.total.Add(v)
			inf.secCost[s] = v
		}
		costs = append(costs, inf)
	}

	keys := make([]string, 0, len(curCost))
	for k := range curCost {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	if len(costs) == 0 {
		return nil
	}

	total := &RenderTable{
		Header: append([]string{"Date", "Total"}, keys...),
	}
	for _, c := range costs {
		var ind []string
		for _, sec := range keys {
			ind = append(ind, ph.DollarStr(c.secCost[sec]))
		}
		total.Rows = append(total.Rows, append([]string{c.date.String(), ph.DollarStr(c.total)}, ind...))
	}

	yearMax := map[int]costinfo{}
	year := costs[0].date.Year()
	var years []int
	var cur costinfo
	dirty := false
	for _, c := range costs {
		if year != c.date.Year() {
			if dirty {
				yearMax[year] = cur
				years = append(years, year)
			}
			year = c.date.Year()
			cur = costinfo{}
		}
		dirty = true
		if cur.total.LessThan(c.total) {
			cur = c
		}

	}
	if dirty {
		yearMax[year] = cur
		years = append(years, year)
	}

	yearly := &RenderTable{
		Header: append([]string{"Year", "Date", "Total"}, keys...),
	}
	for _, year := range years {
		c := yearMax[year]
		var ind []string
		for _, sec := range keys {
			ind = append(ind, ph.DollarStr(c.secCost[sec]))
		}
		yearly.Rows = append(yearly.Rows,
			append([]string{
				fmt.Sprint(year),
				c.date.String(),
				ph.DollarStr(c.total),
			}, ind...),
		)
	}

	return &CostsTables{Total: total, Yearly: yearly}
}
