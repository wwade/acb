package util

import (
	"math/big"
)

func MinValue(val0 big.Rat, vals ...big.Rat) big.Rat {
	min := val0
	for _, v := range vals {
		if v.Cmp(&min) < 0 {
			min = v
		}
	}
	return min
}

func IncRat(x *big.Rat, y big.Rat) {
	x.Add(x, &y)
}

func DecRat(x *big.Rat, y big.Rat) {
	x.Sub(x, &y)
}

func AddRat(x big.Rat, y big.Rat) (ret big.Rat) {
	ret.Add(&x, &y)
	return ret
}
func SubRat(x big.Rat, y big.Rat) (ret big.Rat) {
	ret.Sub(&x, &y)
	return ret
}

func DivRat(x big.Rat, y big.Rat) *big.Rat {
	var ret big.Rat
	ret.Mul(&x, y.Inv(&y))
	return &ret
}

func ToFloat(x big.Rat) float64 {
	v, _ := x.Float64()
	return v
}
