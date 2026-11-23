package semantic

import "math"

// ConservativeMicroUSD is the most one call can cost under this price.
// It is false when the schedule or a token bound is unknown, or when the
// product does not fit in an int64. Callers must not dispatch on false.
func (p Price) ConservativeMicroUSD() (int64, bool) {
	if !p.Known || p.MaxInputTokens < 0 || p.MaxOutputTokens < 0 || p.InputMicroUSDPerToken < 0 || p.OutputMicroUSDPerToken < 0 {
		return 0, false
	}
	in, okIn := mulMicro(p.MaxInputTokens, p.InputMicroUSDPerToken)
	out, okOut := mulMicro(p.MaxOutputTokens, p.OutputMicroUSDPerToken)
	if !okIn || !okOut {
		return 0, false
	}
	return addMicro(in, out)
}

func usageMicroUSD(p Price, resp ModelResponse) (int64, bool) {
	if !resp.TokensKnown || !p.Known || resp.InputTokens < 0 || resp.OutputTokens < 0 {
		return 0, false
	}
	in, okIn := mulMicro(resp.InputTokens, p.InputMicroUSDPerToken)
	out, okOut := mulMicro(resp.OutputTokens, p.OutputMicroUSDPerToken)
	if !okIn || !okOut {
		return 0, false
	}
	return addMicro(in, out)
}

func mulMicro(a, b int64) (int64, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}
	if a == 0 || b == 0 {
		return 0, true
	}
	if a > math.MaxInt64/b {
		return 0, false
	}
	return a * b, true
}

func addMicro(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || a > math.MaxInt64-b {
		return 0, false
	}
	return a + b, true
}
