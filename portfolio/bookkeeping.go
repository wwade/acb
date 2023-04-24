package portfolio

import (
	"fmt"
	"math"
	"math/big"
	"sort"

	"github.com/tsiemens/acb/date"
	"github.com/tsiemens/acb/util"
)

type LegacyOptions struct {
	// None currently
}

func NewLegacyOptions() LegacyOptions {
	return LegacyOptions{}
}

func NonNilTxAffiliate(tx *Tx) *Affiliate {
	txAffiliate := tx.Affiliate
	if txAffiliate == nil {
		// This should only really happen in tests
		txAffiliate = GlobalAffiliateDedupTable.GetDefaultAffiliate()
	}
	return txAffiliate
}

type AffiliatePortfolioSecurityStatuses struct {
	// Affiliate Id -> last PortfolioSecurityStatus
	lastPostStatusForAffiliate      map[string]*PortfolioSecurityStatus
	security                        string
	latestAllAffiliatesShareBalance big.Rat
	latestAffiliate                 *Affiliate
}

func NewAffiliatePortfolioSecurityStatuses(
	security string, initialDefaultAffStatus *PortfolioSecurityStatus,
) *AffiliatePortfolioSecurityStatuses {

	s := &AffiliatePortfolioSecurityStatuses{
		lastPostStatusForAffiliate:      make(map[string]*PortfolioSecurityStatus),
		security:                        security,
		latestAllAffiliatesShareBalance: big.Rat{},
		latestAffiliate:                 GlobalAffiliateDedupTable.GetDefaultAffiliate(),
	}

	// Initial status only applies to the default affiliate
	if initialDefaultAffStatus != nil {
		util.Assert(initialDefaultAffStatus.ShareBalance.Cmp(
			&initialDefaultAffStatus.AllAffiliatesShareBalance) == 0)
		s.SetLatestPostStatus(s.latestAffiliate.Id(), initialDefaultAffStatus)
	}
	return s
}

func (s *AffiliatePortfolioSecurityStatuses) makeDefaultPortfolioSecurityStatus(
	defaultAff bool, registered bool) *PortfolioSecurityStatus {
	var affiliateShareBalance big.Rat
	var affiliateTotalAcb float64 = util.Tern[float64](registered, math.NaN(), 0.0)
	return &PortfolioSecurityStatus{
		Security: s.security, ShareBalance: affiliateShareBalance,
		AllAffiliatesShareBalance: s.latestAllAffiliatesShareBalance,
		TotalAcb:                  affiliateTotalAcb,
	}
}

func (s *AffiliatePortfolioSecurityStatuses) GetLatestPostStatusForAffiliate(
	id string) (*PortfolioSecurityStatus, bool) {
	v, ok := s.lastPostStatusForAffiliate[id]
	return v, ok
}

func (s *AffiliatePortfolioSecurityStatuses) GetLatestPostStatus() *PortfolioSecurityStatus {
	v, ok := s.GetLatestPostStatusForAffiliate(s.latestAffiliate.Id())
	if !ok {
		return &PortfolioSecurityStatus{Security: s.security}
	}
	return v
}

func (s *AffiliatePortfolioSecurityStatuses) SetLatestPostStatus(
	id string, v *PortfolioSecurityStatus) {

	var lastShareBalance big.Rat
	if last, ok := s.lastPostStatusForAffiliate[id]; ok {
		lastShareBalance = last.ShareBalance
	}
	var expectedAllShareBal big.Rat
	expectedAllShareBal.Add(&v.ShareBalance, &s.latestAllAffiliatesShareBalance)
	expectedAllShareBal.Sub(&expectedAllShareBal, &lastShareBalance)

	af := GlobalAffiliateDedupTable.MustGet(id)
	util.Assertf(af.Registered() == math.IsNaN(v.TotalAcb),
		"In security %s, af %s, TotalAcb has bad NaN value (%f)",
		s.security, id, v.TotalAcb)

	util.Assertf(v.AllAffiliatesShareBalance.Cmp(&expectedAllShareBal) == 0,
		"In security %s, af %s, v.AllAffiliatesShareBalance (%v) != expectedAllShareBal (%v) "+
			"(v.ShareBalance (%v) + s.latestAllAffiliatesShareBalance (%v) - lastShareBalance (%v)",
		s.security, id, v.AllAffiliatesShareBalance, expectedAllShareBal,
		v.ShareBalance, s.latestAllAffiliatesShareBalance, lastShareBalance)

	s.lastPostStatusForAffiliate[id] = v
	s.latestAllAffiliatesShareBalance = v.AllAffiliatesShareBalance
	s.latestAffiliate = GlobalAffiliateDedupTable.MustGet(id)
}

func (s *AffiliatePortfolioSecurityStatuses) GetNextPreStatus(
	id string) *PortfolioSecurityStatus {

	lastStatus, ok := s.GetLatestPostStatusForAffiliate(id)
	if !ok {
		af := GlobalAffiliateDedupTable.MustGet(id)
		lastStatus = s.makeDefaultPortfolioSecurityStatus(af.Default(), af.Registered())
	}
	nextPreStatus := lastStatus
	if nextPreStatus.AllAffiliatesShareBalance.Cmp(&s.latestAllAffiliatesShareBalance) != 0 {
		nextPreStatus = &PortfolioSecurityStatus{}
		*nextPreStatus = *lastStatus
		nextPreStatus.AllAffiliatesShareBalance = s.latestAllAffiliatesShareBalance
	}
	return nextPreStatus
}

type _SuperficialLossInfo struct {
	IsSuperficial              bool
	FirstDateInPeriod          date.Date
	LastDateInPeriod           date.Date
	AllAffSharesAtEndOfPeriod  big.Rat
	TotalAquiredInPeriod       big.Rat
	BuyingAffiliates           *util.Set[string]
	ActiveAffiliateSharesAtEOP *util.DefaultMap[string, big.Rat]
}

func (i *_SuperficialLossInfo) BuyingAffiliateSharesAtEOPTotal() big.Rat {
	var total big.Rat
	i.BuyingAffiliates.ForEach(func(afId string) bool {
		v := i.ActiveAffiliateSharesAtEOP.Get(afId)
		total.Add(&total, &v)
		return true
	})
	return total
}

func GetFirstDayInSuperficialLossPeriod(txDate date.Date) date.Date {
	return txDate.AddDays(-30)
}

func GetLastDayInSuperficialLossPeriod(txDate date.Date) date.Date {
	return txDate.AddDays(30)
}

// Checks if there is a Buy action within 30 days before or after the Sell
// at idx, AND if you hold shares after the 30 day period
// Also gathers relevant information for partial superficial loss calculation.
func getSuperficialLossInfo(
	idx int, txs []*Tx, ptfStatuses *AffiliatePortfolioSecurityStatuses) _SuperficialLossInfo {
	tx := txs[idx]
	util.Assertf(tx.Action == SELL,
		"getSuperficialLossInfo: Tx was not Sell, but %s", tx.Action)

	firstBadBuyDate := GetFirstDayInSuperficialLossPeriod(tx.SettlementDate)
	lastBadBuyDate := GetLastDayInSuperficialLossPeriod(tx.SettlementDate)

	latestPostStatus := ptfStatuses.GetLatestPostStatus()
	// The enclosing AddTx logic should have already caught this.
	util.Assertf(latestPostStatus.AllAffiliatesShareBalance.Cmp(&tx.Shares) > 0,
		"getSuperficialLossInfo: latest AllAffiliatesShareBalance (%v) is less than sold shares (%v)",
		latestPostStatus.AllAffiliatesShareBalance, tx.Shares)
	var allAffiliatesShareBalanceAfterSell big.Rat
	allAffiliatesShareBalanceAfterSell.Sub(&ptfStatuses.GetLatestPostStatus().AllAffiliatesShareBalance, &tx.Shares)

	activeAffiliateSharesAtEOP := util.NewDefaultMap[string, big.Rat](
		// Default to post-sale share balance for the affiliate.
		func(afId string) (ret big.Rat) {
			sellTxAffil := NonNilTxAffiliate(tx)
			if st, ok := ptfStatuses.GetLatestPostStatusForAffiliate(afId); ok {
				if afId == sellTxAffil.Id() {
					// The latest post status for the selling affiliate is not yet
					// saved, so recompute the post-sale share balance.
					// AddTx would have encountered an oversell if this was to assert.
					util.Assertf(st.ShareBalance.Cmp(&tx.Shares) > 0,
						"getSuperficialLossInfo: latest ShareBalance (%v) for affiliate (%s) "+
							"is less than sold shares (%v)",
						st.ShareBalance, sellTxAffil.Name(), tx.Shares)
					ret.Sub(&st.ShareBalance, &tx.Shares)
					return ret
				}
				return st.ShareBalance
			}
			// No TXs for this affiliate before or at the current sell.
			// AddTx would have encountered an oversell if this was to assert.
			util.Assertf(afId != sellTxAffil.Id(),
				"getSuperficialLossInfo: no existing portfolio status for affiliate %s",
				sellTxAffil.Name())
			return ret
		})

	sli := _SuperficialLossInfo{
		IsSuperficial:              false,
		FirstDateInPeriod:          firstBadBuyDate,
		LastDateInPeriod:           lastBadBuyDate,
		AllAffSharesAtEndOfPeriod:  allAffiliatesShareBalanceAfterSell,
		TotalAquiredInPeriod:       big.Rat{},
		BuyingAffiliates:           util.NewSet[string](),
		ActiveAffiliateSharesAtEOP: activeAffiliateSharesAtEOP,
	}

	// Some points:
	// the total share balance across all affiliates is insufficient, since
	// if you had 3 affiliates, it's possible to retain shares, but in an affiliate
	// which did not do any of the buys within the period. This should probably
	// require a manual entry, since I don't know what to do in this case. Is the
	// loss denied or not? Is the total number of shares only for the affiliates
	// doing the sell and with the buys?
	// I think the total shares should only be in the affiliates which did the
	// sell.
	// Do we use the shares left in the affiliate with the buy only?
	// hypothetical:
	//  A                 B
	//  BUY 5				BUY 0
	//	 ...              ...
	//  SELL 4 (SFL)		BUY 5
	//							SELL 3
	// (reminaing: 1)		(remaining: 2)
	// use 2 or 3 as remaining shares, since it is the min val for proportional SFL.
	//
	// However, the safer thing to do might be to use the max shares, but require
	// manual entry if the number of shares remaining in the sell affiliate is less
	// than the number of rejected loss shares. <<<<<< Warn of this and possibly suggest an accountant.

	didBuyAfterInPeriod := false
	for i := idx + 1; i < len(txs); i++ {
		afterTx := txs[i]
		if afterTx.SettlementDate.After(lastBadBuyDate) {
			break
		}
		afterTxAffil := NonNilTxAffiliate(afterTx)

		// Within the 30 day window after
		switch afterTx.Action {
		case BUY:
			didBuyAfterInPeriod = true
			util.IncRat(&sli.AllAffSharesAtEndOfPeriod, afterTx.Shares)
			activeAffiliateSharesAtEOP.Set(afterTxAffil.Id(),
				util.AddRat(activeAffiliateSharesAtEOP.Get(afterTxAffil.Id()), afterTx.Shares))
			util.IncRat(&sli.TotalAquiredInPeriod, afterTx.Shares)
			sli.BuyingAffiliates.Add(afterTxAffil.Id())
		case SELL:
			util.DecRat(&sli.AllAffSharesAtEndOfPeriod, afterTx.Shares)
			activeAffiliateSharesAtEOP.Set(afterTxAffil.Id(),
				util.SubRat(activeAffiliateSharesAtEOP.Get(afterTxAffil.Id()), afterTx.Shares))
		default:
			// ignored
		}
	}

	if sli.AllAffSharesAtEndOfPeriod.Sign() == 0 {
		// Not superficial
		return sli
	}

	didBuyBeforeInPeriod := false
	for i := idx - 1; i >= 0; i-- {
		beforeTx := txs[i]
		if beforeTx.SettlementDate.Before(firstBadBuyDate) {
			break
		}
		beforeTxAffil := NonNilTxAffiliate(beforeTx)
		// Within the 30 day window before
		if beforeTx.Action == BUY {
			didBuyBeforeInPeriod = true
			util.IncRat(&sli.TotalAquiredInPeriod, beforeTx.Shares)
			sli.BuyingAffiliates.Add(beforeTxAffil.Id())
		}
	}

	sli.IsSuperficial = didBuyBeforeInPeriod || didBuyAfterInPeriod
	return sli
}

type _SflRatioResultResult struct {
	SflRatio                 *big.Rat
	AcbAdjustAffiliateRatios map[string]*big.Rat
	// ** Notes/warnings to emit later. **
	// Set when the sum of remaining involved affiliate shares is fewer than
	// the SFL shares, which means that the selling affiliate probably had some
	// shares they didn't sell. This can happen because we use interpretation/algo I.1
	// rather than I.2 (see the sfl wiki page) to determine the loss ratio.
	FewerRemainingSharesThanSflShares bool
}

// Calculation of partial superficial losses where
// Superficial loss = (min(#sold, totalAquired, endBalance) / #sold) x (Total Loss)
// This function returns the left hand side of this formula, on the condition that
// the loss is actually superficial.
//
// Returns:
// - the superficial loss ratio (if calculable)
// - the affiliate to apply an automatic adjustment to (if possible)
// - an soft error (warning), which only applies when auto-generating the SfLA
//
// Uses interpretation I.1 from the link below for splitting loss adjustments.
//
// More detailed discussions about adjustment allocation can be found at
// https://github.com/tsiemens/acb/wiki/Superficial-Losses
//
// Reference: https://www.adjustedcostbase.ca/blog/applying-the-superficial-loss-rule-for-a-partial-disposition-of-shares/
func getSuperficialLossRatio(
	idx int, txs []*Tx, ptfStatuses *AffiliatePortfolioSecurityStatuses) *_SflRatioResultResult {
	sli := getSuperficialLossInfo(idx, txs, ptfStatuses)
	if sli.IsSuperficial {
		tx := txs[idx]

		ratio := util.DivRat(
			util.MinValue(tx.Shares, sli.TotalAquiredInPeriod, sli.AllAffSharesAtEndOfPeriod),
			tx.Shares,
		)

		util.Assertf(sli.BuyingAffiliates.Len() != 0,
			"getSuperficialLossRatio: loss was superficial, but no buying affiliates")

		// Affiliate to percentage of the SFL adjustment is attributed to it.
		affiliateAdjustmentPortions := make(map[string]*big.Rat)
		buyingAffilsShareEOPTotal := sli.BuyingAffiliateSharesAtEOPTotal()

		sli.BuyingAffiliates.ForEach(func(afId string) bool {
			afShareBalanceAtEOP := sli.ActiveAffiliateSharesAtEOP.Get(afId)
			affiliateAdjustmentPortions[afId] = util.DivRat(afShareBalanceAtEOP, buyingAffilsShareEOPTotal)
			return true
		})

		numer := big.NewRat(ratio.Num().Int64(), 1)
		return &_SflRatioResultResult{
			SflRatio:                          ratio,
			AcbAdjustAffiliateRatios:          affiliateAdjustmentPortions,
			FewerRemainingSharesThanSflShares: buyingAffilsShareEOPTotal.Cmp(numer) < 0,
		}
	}
	return &_SflRatioResultResult{}
}

// The algorithm to use to determine automatic superficial-loss adjustment
// distribution.
type AutoSflaAlgo int

const (
	// Do not allow automatic SLFA with multiple affiliates.
	SFLA_ALGO_REQUIRE_MANUAL AutoSflaAlgo = iota
	SFLA_ALGO_REJECT_IF_ANY_REGISTERED
	SFLA_ALGO_DISTRIB_BUY_RATIOS
)

type AddTxOptions struct {
	autoSflaAlgo AutoSflaAlgo
}

// Returns a TxDelta for the Tx at txs[idx].
// Optionally, returns a new Tx if a SFLA Tx was generated to accompany
// this Tx. It is expected that that Tx be inserted into txs and evaluated next.
func AddTx(
	idx int,
	txs []*Tx,
	ptfStatuses *AffiliatePortfolioSecurityStatuses,
) (*TxDelta, []*Tx, error) {

	tx := txs[idx]
	txAffiliate := NonNilTxAffiliate(tx)
	preTxStatus := ptfStatuses.GetNextPreStatus(txAffiliate.Id())

	util.Assertf(tx.Security == preTxStatus.Security,
		"AddTx: securities do not match (%s and %s)\n", tx.Security, preTxStatus.Security)

	var totalLocalSharePrice float64 = util.ToFloat(tx.Shares) * tx.AmountPerShare * tx.TxCurrToLocalExchangeRate

	newShareBalance := preTxStatus.ShareBalance
	newAllAffiliatesShareBalance := preTxStatus.AllAffiliatesShareBalance
	registered := tx.Affiliate != nil && tx.Affiliate.Registered()
	var newAcbTotal float64 = preTxStatus.TotalAcb
	var grossIncome float64
	var capitalGains float64 = util.Tern(registered, math.NaN(), 0.0)
	var superficialLoss float64 = util.Tern(registered, math.NaN(), 0.0)
	var superficialLossRatio *big.Rat
	potentiallyOverAppliedSfl := false
	var newTxs []*Tx = nil

	// Sanity checks
	sanityCheckError := func(fmtStr string, v ...interface{}) error {
		return fmt.Errorf(
			"In transaction on %v of %v shares of %s, "+fmtStr,
			append([]interface{}{tx.TradeDate, tx.Shares, tx.Security}, v...)...)
	}
	if preTxStatus.AllAffiliatesShareBalance.Cmp(&preTxStatus.ShareBalance) < 0 {
		return nil, nil, sanityCheckError("the share balance across all affiliates "+
			"(%v) is lower than the share balance for the affiliate of the sale (%v)",
			preTxStatus.AllAffiliatesShareBalance, preTxStatus.ShareBalance)
	} else if registered && !math.IsNaN(preTxStatus.TotalAcb) {
		return nil, nil, sanityCheckError("found an ACB on a registered affiliate")
	} else if !registered && math.IsNaN(preTxStatus.TotalAcb) {
		return nil, nil, sanityCheckError("found an invalid ACB (NaN)")
	}

	switch tx.Action {
	case BUY:
		newShareBalance = util.AddRat(preTxStatus.ShareBalance, tx.Shares)
		newAllAffiliatesShareBalance = util.AddRat(preTxStatus.AllAffiliatesShareBalance, tx.Shares)
		totalPrice := totalLocalSharePrice + (tx.Commission * tx.CommissionCurrToLocalExchangeRate)
		newAcbTotal = preTxStatus.TotalAcb + (totalPrice)
	case SELL:
		if tx.Shares.Cmp(&preTxStatus.ShareBalance) > 0 {
			return nil, nil, fmt.Errorf(
				"Sell order on %v of %v shares of %s is more than the current holdings (%v)",
				tx.TradeDate, tx.Shares, tx.Security, preTxStatus.ShareBalance)
		}
		newShareBalance = util.SubRat(preTxStatus.ShareBalance, tx.Shares)
		newAllAffiliatesShareBalance = util.SubRat(preTxStatus.AllAffiliatesShareBalance, tx.Shares)
		// Note commission plays no effect on sell order ACB
		newAcbTotal = preTxStatus.TotalAcb - (preTxStatus.PerShareAcb() * util.ToFloat(tx.Shares))
		totalPayout := totalLocalSharePrice - (tx.Commission * tx.CommissionCurrToLocalExchangeRate)
		grossIncome = totalPayout
		capitalGains = totalPayout - (preTxStatus.PerShareAcb() * util.ToFloat(tx.Shares))

		if !registered && capitalGains < 0.0 {
			sflRatioResult := getSuperficialLossRatio(idx, txs, ptfStatuses)
			superficialLossRatio = sflRatioResult.SflRatio
			calculatedSuperficialLoss := 0.0
			if superficialLossRatio != nil {
				calculatedSuperficialLoss = capitalGains * util.ToFloat(*superficialLossRatio)
			}

			if tx.SpecifiedSuperficialLoss.Present() {
				superficialLoss = tx.SpecifiedSuperficialLoss.MustGet().SuperficialLoss
				capitalGains = capitalGains - superficialLoss

				if !tx.SpecifiedSuperficialLoss.MustGet().Force {
					sflDiff := math.Abs(calculatedSuperficialLoss - superficialLoss)
					const maxDiff float64 = 0.001
					if sflDiff > maxDiff {
						return nil, nil, fmt.Errorf(
							"Sell order on %v of %s: superficial loss was specified, but "+
								"the difference between the specified value (%f) and the "+
								"computed value (%f) is greater than the max allowed "+
								"discrepancy (%f).\nTo force this SFL value, append an '!' "+
								"to the value",
							tx.TradeDate, tx.Security, superficialLoss,
							calculatedSuperficialLoss, maxDiff)
					}
				}

				// ACB adjustment TX must be specified manually in this case.
			} else if superficialLossRatio != nil {
				util.Assert(sflRatioResult.AcbAdjustAffiliateRatios != nil,
					"addTx: sflRatioResult.AcbAdjustAffiliateRatios was nil")
				util.Assert(len(sflRatioResult.AcbAdjustAffiliateRatios) > 0,
					"addTx: sflRatioResult.AcbAdjustAffiliateRatios was empty")

				superficialLoss = calculatedSuperficialLoss
				capitalGains = capitalGains - calculatedSuperficialLoss
				potentiallyOverAppliedSfl = sflRatioResult.FewerRemainingSharesThanSflShares

				acbAdjustAffiliates := util.MapKeys[string, *big.Rat](sflRatioResult.AcbAdjustAffiliateRatios)
				sort.Strings(acbAdjustAffiliates)
				for _, afId := range acbAdjustAffiliates {
					ratioOfSfl := sflRatioResult.AcbAdjustAffiliateRatios[afId]
					autoAdjustAffiliate := GlobalAffiliateDedupTable.MustGet(afId)
					if ratioOfSfl != nil && !autoAdjustAffiliate.Registered() {
						// This new Tx will adjust (increase) the ACB for this superficial loss.
						newTxs = append(newTxs, &Tx{
							Security:                  tx.Security,
							TradeDate:                 tx.TradeDate,
							SettlementDate:            tx.SettlementDate,
							Action:                    SFLA,
							Shares:                    *big.NewRat(1, 1),
							AmountPerShare:            -1.0 * superficialLoss * util.ToFloat(*ratioOfSfl),
							TxCurrency:                CAD,
							TxCurrToLocalExchangeRate: 1.0,
							Memo: fmt.Sprintf(
								"Automatic SfL ACB adjustment. %.2f%% (%v/%v) of SfL, which was %v/%v of sale shares.",
								util.ToFloat(*ratioOfSfl)*100.0, ratioOfSfl.Num(), ratioOfSfl.Denom(),
								superficialLossRatio.Num(), superficialLossRatio.Denom(),
							),
							Affiliate: autoAdjustAffiliate,
						})
					}
				}
			}
		} else if tx.SpecifiedSuperficialLoss.Present() {
			return nil, nil, fmt.Errorf(
				"Sell order on %v of %s: superficial loss was specified, but there is no capital loss",
				tx.TradeDate, tx.Security)
		}
	case ROC:
		if registered {
			return nil, nil, fmt.Errorf(
				"Invalid RoC tx on %v: Registered affiliates do not have an ACB to adjust",
				tx.TradeDate)
		}
		if tx.Shares.Sign() != 0 {
			return nil, nil, fmt.Errorf("Invalid RoC tx on %v: # of shares is non-zero (%v)",
				tx.TradeDate, tx.Shares)
		}
		acbReduction := tx.AmountPerShare * util.ToFloat(preTxStatus.ShareBalance) *
			tx.TxCurrToLocalExchangeRate
		newAcbTotal = preTxStatus.TotalAcb - acbReduction
		if newAcbTotal < 0.0 {
			return nil, nil, fmt.Errorf("Invalid RoC tx on %v: RoC (%f) exceeds the current ACB (%f)",
				tx.TradeDate, acbReduction, preTxStatus.TotalAcb)
		}
	case SFLA:
		if registered {
			return nil, nil, fmt.Errorf(
				"Invalid SfLA tx on %v: Registered affiliates do not have an ACB to adjust",
				tx.TradeDate)
		}
		acbAdjustment := tx.AmountPerShare * util.ToFloat(tx.Shares) *
			tx.TxCurrToLocalExchangeRate
		newAcbTotal = preTxStatus.TotalAcb + acbAdjustment
		if !(tx.TxCurrency == CAD || tx.TxCurrency == DEFAULT_CURRENCY) ||
			tx.TxCurrToLocalExchangeRate != 1.0 {
			return nil, nil, fmt.Errorf(
				"Invalid SfLA tx on %v: Currency is not CAD/default, and/or exchange rate is not 1",
				tx.TradeDate)
		}
	default:
		util.Assertf(false, "Invalid action: %v\n", tx.Action)
	}

	newStatus := &PortfolioSecurityStatus{
		Security:                  preTxStatus.Security,
		ShareBalance:              newShareBalance,
		AllAffiliatesShareBalance: newAllAffiliatesShareBalance,
		TotalAcb:                  newAcbTotal,
	}
	delta := &TxDelta{
		Tx:                        tx,
		PreStatus:                 preTxStatus,
		PostStatus:                newStatus,
		CapitalGain:               capitalGains,
		GrossIncome:               grossIncome,
		SuperficialLoss:           superficialLoss,
		SuperficialLossRatio:      superficialLossRatio,
		PotentiallyOverAppliedSfl: potentiallyOverAppliedSfl,
	}
	return delta, newTxs, nil
}

// Insert tx at index i and return the resulting slice
func insertTx(slice []*Tx, tx *Tx, i int) []*Tx {
	newSlice := make([]*Tx, 0, len(slice)+1)
	newSlice = append(newSlice, slice[:i]...)
	newSlice = append(newSlice, tx)
	newSlice = append(newSlice, slice[i:]...)
	return newSlice
}

func TxsToDeltaList(
	txs []*Tx,
	initialStatus *PortfolioSecurityStatus,
	legacyOptions LegacyOptions,
) ([]*TxDelta, error) {

	var modifiedTxs []*Tx
	activeTxs := txs
	deltas := make([]*TxDelta, 0, len(txs))

	if len(txs) == 0 {
		return deltas, nil
	}

	ptfStatuses := NewAffiliatePortfolioSecurityStatuses(
		txs[0].Security, initialStatus)

	for i := 0; i < len(activeTxs); i++ {
		txAffiliate := NonNilTxAffiliate(activeTxs[i])
		delta, newTxs, err := AddTx(i, activeTxs, ptfStatuses)
		if err != nil {
			// Return what we've managed so far, for debugging
			return deltas, err
		}
		ptfStatuses.SetLatestPostStatus(txAffiliate.Id(), delta.PostStatus)
		deltas = append(deltas, delta)
		if newTxs != nil {
			// Add new Tx into modifiedTxs
			if modifiedTxs == nil {
				// Copy Txs, as we now need to modify
				modifiedTxs = make([]*Tx, 0, len(txs))
				modifiedTxs = append(modifiedTxs, txs...)
				activeTxs = modifiedTxs
			}
			// Insert into modifiedTxs after the current Tx
			for newTxI, newTx := range newTxs {
				modifiedTxs = insertTx(modifiedTxs, newTx, i+newTxI+1)
			}
			activeTxs = modifiedTxs
		}
	}
	return deltas, nil
}

func SplitTxsBySecurity(txs []*Tx) map[string][]*Tx {
	txsBySec := make(map[string][]*Tx)
	for _, tx := range txs {
		secTxs, ok := txsBySec[tx.Security]
		if !ok {
			secTxs = make([]*Tx, 0, 8)
		}
		secTxs = append(secTxs, tx)
		txsBySec[tx.Security] = secTxs
	}
	return txsBySec
}
