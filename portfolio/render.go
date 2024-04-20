package portfolio

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	decimal_opt "github.com/tsiemens/acb/decimal_value"
	"github.com/tsiemens/acb/util"
)

type PrintHelper struct {
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

func (h PrintHelper) CurrStr(val decimal.Decimal) string {
	v, err := strconv.ParseFloat(val.String(), 64)
	if err != nil {
		panic(err)
	}
	p := message.NewPrinter(language.English)
	if h.PrintAllDecimals {
		return p.Sprintf("%f", v)
	}
	return p.Sprintf("%.2f", v)
}

func (h PrintHelper) OptCurrStr(val decimal_opt.DecimalOpt) string {
	if val.IsNull {
		return val.String()
	}
	return h.CurrStr(val.Decimal)
}

func (h PrintHelper) DollarStr(val decimal_opt.DecimalOpt) string {
	if val.IsNull {
		return NaNString()
	}
	return "$" + h.OptCurrStr(val)
}

func (h PrintHelper) CurrWithFxStr(val decimal.Decimal, curr Currency, rateToLocal decimal.Decimal) string {
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
func (h PrintHelper) PlusMinusDollar(val decimal_opt.DecimalOpt, showPlus bool) string {
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

func RenderTxTableModel(
	deltas []*TxDelta, gains *CumulativeCapitalGains, renderFullDollarValues bool) *RenderTable {
	table := &RenderTable{}
	table.Header = []string{"Security", "Trade Date", "Settl. Date", "TX", "Amount", "Shares", "Amt/Share", "ACB",
		"Commission", "Cap. Gain", "Share Balance", "ACB +/-", "New ACB", "New ACB/Share",
		"Affiliate", "Memo",
	}

	ph := PrintHelper{PrintAllDecimals: renderFullDollarValues}

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
				util.Tern[string](specifiedSflIsForced, "!", ""),
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

	ph := PrintHelper{PrintAllDecimals: renderFullDollarValues}

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

func PrintRenderTable(title string, tableModel *RenderTable, writer io.Writer) {
	for _, err := range tableModel.Errors {
		fmt.Fprintf(writer, "[!] %v. Printing parsed information state:\n", err)
	}
	if title != "" {
		fmt.Fprintf(writer, "%s\n", title)
	}

	o := csv.NewWriter(writer)
	o.Write(tableModel.Header)

	for _, row := range tableModel.Rows {
		o.Write(row)
	}
	if len(tableModel.Footer) > 0 {
		o.Write(tableModel.Footer)
	}
	o.Flush()

	for _, note := range tableModel.Notes {
		fmt.Fprintln(writer, note)
	}
}
