package services

import (
	"math"
)

// IndicatorResult holds all calculated technical indicators for a single price point
type IndicatorResult struct {
	// Moving Averages
	MA5   *float64 `json:"ma5,omitempty"`
	MA10  *float64 `json:"ma10,omitempty"`
	MA20  *float64 `json:"ma20,omitempty"`
	MA60  *float64 `json:"ma60,omitempty"`
	MA120 *float64 `json:"ma120,omitempty"`

	// Exponential Moving Average
	EMA12 *float64 `json:"ema12,omitempty"`
	EMA26 *float64 `json:"ema26,omitempty"`

	// MACD
	MACD       *float64 `json:"macd,omitempty"`
	MACDSignal *float64 `json:"macd_signal,omitempty"`
	MACDHist   *float64 `json:"macd_histogram,omitempty"`

	// RSI
	RSI14 *float64 `json:"rsi14,omitempty"`

	// Bollinger Bands
	BBUpper  *float64 `json:"bb_upper,omitempty"`
	BBMiddle *float64 `json:"bb_middle,omitempty"`
	BBLower  *float64 `json:"bb_lower,omitempty"`

	// KDJ
	KDJK *float64 `json:"kdj_k,omitempty"`
	KDJD *float64 `json:"kdj_d,omitempty"`
	KDJJ *float64 `json:"kdj_j,omitempty"`

	// ATR
	ATR14 *float64 `json:"atr14,omitempty"`
}

// CalculateMA calculates Simple Moving Average
func CalculateMA(prices []float64, period int) []float64 {
	if len(prices) < period || period <= 0 {
		return make([]float64, len(prices))
	}

	result := make([]float64, len(prices))
	for i := 0; i < len(prices); i++ {
		if i < period-1 {
			result[i] = math.NaN()
			continue
		}
		sum := 0.0
		for j := 0; j < period; j++ {
			sum += prices[i-period+1+j]
		}
		result[i] = sum / float64(period)
	}
	return result
}

// CalculateEMA calculates Exponential Moving Average
func CalculateEMA(prices []float64, period int) []float64 {
	if len(prices) < period || period <= 0 {
		return make([]float64, len(prices))
	}

	result := make([]float64, len(prices))
	multiplier := 2.0 / (float64(period) + 1)

	// First EMA is SMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	result[period-1] = sum / float64(period)

	// Subsequent EMAs
	for i := period; i < len(prices); i++ {
		result[i] = (prices[i] * multiplier) + (result[i-1] * (1 - multiplier))
	}

	// Fill earlier values with NaN
	for i := 0; i < period-1; i++ {
		result[i] = math.NaN()
	}

	return result
}

// CalculateMACD calculates MACD, Signal line, and Histogram
func CalculateMACD(prices []float64) (macd, signal, histogram []float64) {
	if len(prices) < 26 {
		return make([]float64, len(prices)), make([]float64, len(prices)), make([]float64, len(prices))
	}

	ema12 := CalculateEMA(prices, 12)
	ema26 := CalculateEMA(prices, 26)

	macd = make([]float64, len(prices))
	for i := 0; i < len(prices); i++ {
		if i < 25 {
			macd[i] = math.NaN()
		} else {
			macd[i] = ema12[i] - ema26[i]
		}
	}

	signal = CalculateEMA(macd, 9)

	histogram = make([]float64, len(prices))
	for i := 0; i < len(prices); i++ {
		if math.IsNaN(macd[i]) || math.IsNaN(signal[i]) {
			histogram[i] = math.NaN()
		} else {
			histogram[i] = macd[i] - signal[i]
		}
	}

	return
}

// CalculateRSI calculates Relative Strength Index
func CalculateRSI(prices []float64, period int) []float64 {
	if len(prices) < period+1 || period <= 0 {
		return make([]float64, len(prices))
	}

	result := make([]float64, len(prices))

	// Calculate gains and losses
	gains := make([]float64, len(prices)-1)
	losses := make([]float64, len(prices)-1)

	for i := 1; i < len(prices); i++ {
		change := prices[i] - prices[i-1]
		if change > 0 {
			gains[i-1] = change
			losses[i-1] = 0
		} else {
			gains[i-1] = 0
			losses[i-1] = -change
		}
	}

	// Calculate average gains and losses (only for valid indices)
	avgGain := make([]float64, len(gains))
	avgLoss := make([]float64, len(losses))

	// First value is simple average
	sumGain := 0.0
	sumLoss := 0.0
	for i := 0; i < period && i < len(gains); i++ {
		sumGain += gains[i]
		sumLoss += losses[i]
	}
	avgGain[period-1] = sumGain / float64(period)
	avgLoss[period-1] = sumLoss / float64(period)

	// Subsequent values use exponential smoothing
	for i := period; i < len(gains); i++ {
		avgGain[i] = (avgGain[i-1]*float64(period-1) + gains[i]) / float64(period)
		avgLoss[i] = (avgLoss[i-1]*float64(period-1) + losses[i]) / float64(period)
	}

	// Calculate RSI
	for i := 0; i < len(result); i++ {
		if i < period {
			result[i] = math.NaN()
		} else {
			// Make sure we're accessing valid indices
			if i-1 < len(avgGain) && i-1 < len(avgLoss) {
				rs := avgGain[i-1] / avgLoss[i-1]
				if avgLoss[i-1] != 0 {
					result[i] = 100 - (100 / (1 + rs))
				} else {
					result[i] = 100
				}
			} else {
				result[i] = math.NaN()
			}
		}
	}

	return result
}

// CalculateBollingerBands calculates Bollinger Bands (upper, middle, lower)
func CalculateBollingerBands(prices []float64, period int, stdDev float64) (upper, middle, lower []float64) {
	if len(prices) < period || period <= 0 {
		return make([]float64, len(prices)), make([]float64, len(prices)), make([]float64, len(prices))
	}

	upper = make([]float64, len(prices))
	middle = make([]float64, len(prices))
	lower = make([]float64, len(prices))

	// Middle band is SMA
	sma := CalculateMA(prices, period)

	for i := 0; i < len(prices); i++ {
		if i < period-1 {
			upper[i] = math.NaN()
			middle[i] = math.NaN()
			lower[i] = math.NaN()
			continue
		}

		middle[i] = sma[i]

		// Calculate standard deviation
		sumSq := 0.0
		for j := 0; j < period; j++ {
			diff := prices[i-period+1+j] - sma[i]
			sumSq += diff * diff
		}
		std := math.Sqrt(sumSq / float64(period))

		upper[i] = middle[i] + (stdDev * std)
		lower[i] = middle[i] - (stdDev * std)
	}

	return
}

// CalculateKDJ calculates KDJ indicator (Stochastic Oscillator variant)
func CalculateKDJ(highs, lows, closes []float64, period int) (kdjK, kdjD, kdjJ []float64) {
	if len(highs) < period || len(lows) < period || len(closes) < period || period <= 0 {
		return make([]float64, len(closes)), make([]float64, len(closes)), make([]float64, len(closes))
	}

	kdjK = make([]float64, len(closes))
	kdjD = make([]float64, len(closes))
	kdjJ = make([]float64, len(closes))

	// Calculate raw stochastic
	rawK := make([]float64, len(closes))
	for i := 0; i < len(closes); i++ {
		if i < period-1 {
			rawK[i] = math.NaN()
			continue
		}

		minLow := lows[i]
		maxHigh := highs[i]
		for j := 1; j < period; j++ {
			if lows[i-j] < minLow {
				minLow = lows[i-j]
			}
			if highs[i-j] > maxHigh {
				maxHigh = highs[i-j]
			}
		}

		if maxHigh-minLow == 0 {
			rawK[i] = 50
		} else {
			rawK[i] = ((closes[i] - minLow) / (maxHigh - minLow)) * 100
		}
	}

	// Smooth K with 3-period MA, D with 3-period MA of K
	smoothK := CalculateMA(rawK, 3)
	smoothD := CalculateMA(smoothK, 3)

	for i := 0; i < len(closes); i++ {
		if !math.IsNaN(smoothK[i]) {
			kdjK[i] = smoothK[i]
		} else {
			kdjK[i] = math.NaN()
		}

		if !math.IsNaN(smoothD[i]) {
			kdjD[i] = smoothD[i]
		} else {
			kdjD[i] = math.NaN()
		}

		if !math.IsNaN(kdjK[i]) && !math.IsNaN(kdjD[i]) {
			kdjJ[i] = 3*kdjK[i] - 2*kdjD[i]
		} else {
			kdjJ[i] = math.NaN()
		}
	}

	return
}

// CalculateATR calculates Average True Range
func CalculateATR(highs, lows, closes []float64, period int) []float64 {
	if len(highs) < period || len(lows) < period || len(closes) < period || period <= 0 {
		return make([]float64, len(closes))
	}

	// Calculate True Range
	tr := make([]float64, len(closes))
	for i := 0; i < len(closes); i++ {
		if i == 0 {
			tr[i] = highs[i] - lows[i]
		} else {
			hl := highs[i] - lows[i]
			hc := math.Abs(highs[i] - closes[i-1])
			lc := math.Abs(lows[i] - closes[i-1])

			max := hl
			if hc > max {
				max = hc
			}
			if lc > max {
				max = lc
			}
			tr[i] = max
		}
	}

	// Calculate ATR using EMA
	atr := make([]float64, len(closes))
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += tr[i]
	}
	atr[period-1] = sum / float64(period)

	for i := period; i < len(closes); i++ {
		atr[i] = (atr[i-1]*float64(period-1) + tr[i]) / float64(period)
	}

	// Fill earlier values with NaN
	for i := 0; i < period-1; i++ {
		atr[i] = math.NaN()
	}

	return atr
}

// CalculateAllIndicators calculates all indicators for a price series and returns results for each point
func CalculateAllIndicators(closes []float64, highs []float64, lows []float64) []IndicatorResult {
	if len(closes) == 0 {
		return []IndicatorResult{}
	}

	results := make([]IndicatorResult, len(closes))

	// Calculate all indicators
	ma5 := CalculateMA(closes, 5)
	ma10 := CalculateMA(closes, 10)
	ma20 := CalculateMA(closes, 20)
	ma60 := CalculateMA(closes, 60)
	ma120 := CalculateMA(closes, 120)

	ema12 := CalculateEMA(closes, 12)
	ema26 := CalculateEMA(closes, 26)

	macd, signal, histogram := CalculateMACD(closes)
	rsi14 := CalculateRSI(closes, 14)

	bbUpper, bbMiddle, bbLower := CalculateBollingerBands(closes, 20, 2)

	kdjK, kdjD, kdjJ := CalculateKDJ(highs, lows, closes, 9)
	atr14 := CalculateATR(highs, lows, closes, 14)

	// Combine results
	for i := 0; i < len(closes); i++ {
		result := IndicatorResult{}

		if !math.IsNaN(ma5[i]) {
			v := ma5[i]
			result.MA5 = &v
		}
		if !math.IsNaN(ma10[i]) {
			v := ma10[i]
			result.MA10 = &v
		}
		if !math.IsNaN(ma20[i]) {
			v := ma20[i]
			result.MA20 = &v
		}
		if !math.IsNaN(ma60[i]) {
			v := ma60[i]
			result.MA60 = &v
		}
		if !math.IsNaN(ma120[i]) {
			v := ma120[i]
			result.MA120 = &v
		}

		if !math.IsNaN(ema12[i]) {
			v := ema12[i]
			result.EMA12 = &v
		}
		if !math.IsNaN(ema26[i]) {
			v := ema26[i]
			result.EMA26 = &v
		}

		if !math.IsNaN(macd[i]) {
			v := macd[i]
			result.MACD = &v
		}
		if !math.IsNaN(signal[i]) {
			v := signal[i]
			result.MACDSignal = &v
		}
		if !math.IsNaN(histogram[i]) {
			v := histogram[i]
			result.MACDHist = &v
		}

		if !math.IsNaN(rsi14[i]) {
			v := rsi14[i]
			result.RSI14 = &v
		}

		if !math.IsNaN(bbUpper[i]) {
			v := bbUpper[i]
			result.BBUpper = &v
		}
		if !math.IsNaN(bbMiddle[i]) {
			v := bbMiddle[i]
			result.BBMiddle = &v
		}
		if !math.IsNaN(bbLower[i]) {
			v := bbLower[i]
			result.BBLower = &v
		}

		if !math.IsNaN(kdjK[i]) {
			v := kdjK[i]
			result.KDJK = &v
		}
		if !math.IsNaN(kdjD[i]) {
			v := kdjD[i]
			result.KDJD = &v
		}
		if !math.IsNaN(kdjJ[i]) {
			v := kdjJ[i]
			result.KDJJ = &v
		}

		if !math.IsNaN(atr14[i]) {
			v := atr14[i]
			result.ATR14 = &v
		}

		results[i] = result
	}

	return results
}
