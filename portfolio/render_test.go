package portfolio

import (
	"os"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestLocaleStringAll(t *testing.T) {
	require.NoError(t, os.Setenv("HUMANIZE", "1"))

	for _, tc := range []struct {
		orig string
		exp  string
	}{
		{
			orig: "10",
			exp:  "10.00",
		},
		{
			orig: "1123",
			exp:  "1,123.00",
		},
		{
			orig: "99991123",
			exp:  "99,991,123.00",
		},
		{
			orig: ".3",
			exp:  "0.30",
		},
		{
			orig: "0.3",
			exp:  "0.30",
		},
		{
			orig: "1.235567",
			exp:  "1.24",
		},
		{
			orig: "123.234567",
			exp:  "123.23",
		},
		{
			orig: "1234.234567",
			exp:  "1,234.23",
		},
		{
			orig: "12345678.234567",
			exp:  "12,345,678.23",
		},
	} {
		for _, negative := range []string{"", "-"} {
			value := negative + tc.orig
			t.Run(value, func(t *testing.T) {
				h := _PrintHelper{PrintAllDecimals: false}
				dec, err := decimal.NewFromString(value)
				require.NoError(t, err)
				v := h.CurrStr(dec)
				expected := negative + tc.exp
				t.Log("orig:", tc.orig)
				t.Log("expected:", tc.exp)
				t.Log("negative:", negative)
				require.Equal(t, expected, v)
			})
		}
	}
}
