package cluster

import (
	"testing"
)

func TestSortScores(t *testing.T) {
	s := []*score{
		{r: nil, score: 1},
		{r: nil, score: 2},
		{r: nil, score: 3},
		{r: nil, score: 9},
	}

	sortScores(s)

	first := s[0]
	if first.score != 1.0 {
		t.Fatalf("expected first score to be 1.0 received %f", first.score)
	}

	last := s[len(s)-1]
	if last.score != 9.0 {
		t.Fatalf("expected last score to be 9.0 received %f", last.score)
	}
}
