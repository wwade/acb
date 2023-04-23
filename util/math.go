package util

import "golang.org/x/exp/constraints"

func MinValue[T constraints.Ordered](val0 T, vals ...T) T {
	min := val0
	for _, v := range vals {
		if v < min {
			min = v
		}
	}
	return min
}

type RatioF64 struct {
	Numerator   float64
	Denominator float64
}

func (r *RatioF64) Valid() bool {
	return r.Denominator != 0
}

func (r *RatioF64) ToFloat64() float64 {
	return r.Numerator / r.Denominator
}

func AlmostEqual[T constraints.Float](v0, v1 T) bool {
	const threshold = 1e-9
	if v0 > v1 {
		return (v0 - v1) < threshold
	} else {
		return (v1 - v0) < threshold
	}
}
