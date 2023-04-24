package portfolio

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	tw "github.com/olekukonko/tablewriter"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

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

func (h PrintHelper) CurrStr(val float64) string {
	p := message.NewPrinter(language.English)
	if h.PrintAllDecimals {
		return p.Sprintf("%f", val)
	}
	return p.Sprintf("%.2f", val)
}

func (h PrintHelper) DollarStr(val float64) string {
	if math.IsNaN(val) {
		return NaNString()
	}
	return "$" + h.CurrStr(val)
}

func (h PrintHelper) CurrWithFxStr(val float64, curr Currency, rateToLocal float64) string {
	if curr == DEFAULT_CURRENCY {
		return h.DollarStr(val)
	}
	return fmt.Sprintf("%s\n(%s %s)", h.DollarStr(val*rateToLocal), h.CurrStr(val), curr)
}

func strOrDash(useStr bool, str string) string {
	if useStr {
		return str
	}
	return "-"
}

func (h PrintHelper) PlusMinusDollar(val float64, showPlus bool) string {
	if math.IsNaN(val) {
		return NaNString()
	}
	if val < 0.0 {
		return fmt.Sprintf("-$%s", h.CurrStr(val*-1.0))
	}
	plus := ""
	if showPlus {
		plus = "+"
	}
	return fmt.Sprintf("%s$%s", plus, h.CurrStr(val))
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
		if d.SuperficialLoss != 0.0 && !math.IsNaN(d.SuperficialLoss) {
			extraSflNoteStr := ""
			if d.PotentiallyOverAppliedSfl {
				extraSflNoteStr = " [1]"
			}

			superficialLossAsterix = fmt.Sprintf(
				" *\n(SfL %s%s; %v/%v%s)",
				ph.PlusMinusDollar(d.SuperficialLoss, false),
				util.Tern[string](specifiedSflIsForced, "!", ""),
				d.SuperficialLossRatio.Num(),
				d.SuperficialLossRatio.Denom(),
				extraSflNoteStr,
			)
			sawSuperficialLoss = true
			sawOverAppliedSfl = sawOverAppliedSfl || d.PotentiallyOverAppliedSfl
		}
		tx := d.Tx

		var preAcbPerShare float64 = 0.0
		if tx.Action == SELL && d.PreStatus.ShareBalance.Sign() > 0 {
			preAcbPerShare = d.PreStatus.TotalAcb / util.ToFloat(d.PreStatus.ShareBalance)
		}

		var affiliateName string
		if tx.Affiliate != nil {
			affiliateName = tx.Affiliate.Name()
		} else {
			affiliateName = GlobalAffiliateDedupTable.GetDefaultAffiliate().Name()
		}

		row := []string{d.Tx.Security, tx.TradeDate.String(), tx.SettlementDate.String(), tx.Action.String(),
			// Amount
			ph.CurrWithFxStr(util.ToFloat(tx.Shares)*tx.AmountPerShare, tx.TxCurrency, tx.TxCurrToLocalExchangeRate),
			fmt.Sprintf("%v", tx.Shares),
			ph.CurrWithFxStr(tx.AmountPerShare, tx.TxCurrency, tx.TxCurrToLocalExchangeRate),
			// ACB of sale
			strOrDash(tx.Action == SELL, ph.DollarStr(preAcbPerShare*util.ToFloat(tx.Shares))),
			// Commission
			strOrDash(tx.Commission != 0.0,
				ph.CurrWithFxStr(tx.Commission, tx.CommissionCurrency, tx.CommissionCurrToLocalExchangeRate)),
			// Cap gains
			strOrDash(tx.Action == SELL, ph.PlusMinusDollar(d.CapitalGain, false)+superficialLossAsterix),
			util.Tern(d.PostStatus.ShareBalance.Cmp(&d.PostStatus.AllAffiliatesShareBalance) != 0,
				fmt.Sprintf("%v / %v", d.PostStatus.ShareBalance, d.PostStatus.AllAffiliatesShareBalance),
				fmt.Sprintf("%v", d.PostStatus.ShareBalance)),
			ph.PlusMinusDollar(d.AcbDelta(), true),
			ph.DollarStr(d.PostStatus.TotalAcb),
			// Acb per share
			strOrDash(d.PostStatus.ShareBalance.Sign() != 0,
				ph.DollarStr(d.PostStatus.TotalAcb/util.ToFloat(d.PostStatus.ShareBalance))),
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
	fmt.Fprintf(writer, "%s\n", title)

	table := tw.NewWriter(writer)
	table.SetHeader(tableModel.Header)
	table.SetBorder(false)
	table.SetRowLine(true)

	for _, row := range tableModel.Rows {
		table.Append(row)
	}

	table.SetFooter(tableModel.Footer)

	table.Render()

	for _, note := range tableModel.Notes {
		fmt.Fprintln(writer, note)
	}
}
