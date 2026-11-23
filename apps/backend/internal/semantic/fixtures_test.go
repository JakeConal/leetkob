package semantic

import (
	"fmt"
	"strings"
)

func samplePrice() Price {
	return Price{
		Known:                  true,
		MaxInputTokens:         1000,
		MaxOutputTokens:        100,
		InputMicroUSDPerToken:  1,
		OutputMicroUSDPerToken: 1,
	}
}

func (p Price) mustMax() int64 {
	n, ok := p.ConservativeMicroUSD()
	if !ok {
		panic("fixture price must be known")
	}
	return n
}

func sampleInput() Input {
	return Input{
		LabID:       "lab-1",
		Statement:   "Sort the integers.",
		Constraints: []Constraint{{ID: "c1", Text: "Use a merge sort you wrote."}},
		Source:      "int main(){return 0;}\n",
		Price:       samplePrice(),
	}
}

func passJSON(ids ...string) string {
	var b strings.Builder
	b.WriteString(`{"results":[`)
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"constraintId":%q,"verdict":"pass","evidence":[]}`, id)
	}
	b.WriteString(`]}`)
	return b.String()
}
