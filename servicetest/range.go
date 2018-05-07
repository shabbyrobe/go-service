package servicetest

import (
	"math/rand"
	"time"
)

type TimeRange struct {
	Min time.Duration
	Max time.Duration
}

func (t TimeRange) Rand() time.Duration {
	return randDuration(t.Min, t.Max)
}

type TimeRangeMaker struct {
	Min TimeRange
	Max TimeRange
}

func (t TimeRangeMaker) Rand() TimeRange {
	return TimeRange{Min: t.Min.Rand(), Max: t.Max.Rand()}
}

type IntRange struct {
	Min int
	Max int
}

func (t IntRange) Rand() int {
	return rand.Intn((t.Max+1)-t.Min) + t.Min
}

type IntRangeMaker struct {
	Min IntRange
	Max IntRange
}

func (t IntRangeMaker) Rand() IntRange {
	return IntRange{Min: t.Min.Rand(), Max: t.Max.Rand()}
}

type FloatRange struct {
	Min float64
	Max float64
}

func (t FloatRange) Rand() float64 {
	f := rand.Float64()
	top := (t.Max - t.Min) * f
	return top + t.Min
}

type FloatRangeMaker struct {
	Min FloatRange
	Max FloatRange
}

func (t FloatRangeMaker) Rand() FloatRange {
	return FloatRange{Min: t.Min.Rand(), Max: t.Max.Rand()}
}

func should(chance float64) bool {
	if chance <= 0 {
		return false
	} else if chance >= 1 {
		return true
	}
	max := uint64(1000000)
	next := float64(rand.Uint64() % max)
	return next < (chance * float64(max))
}

func randDuration(min, max time.Duration) time.Duration {
	if min == 0 && max == 0 {
		return 0
	} else if min == max {
		return min
	}
	return time.Duration(rand.Int63n(int64(max)-int64(min))) + min
}
