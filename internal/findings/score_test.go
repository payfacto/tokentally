package findings

import "testing"

func TestScore_CleanSessionIsA(t *testing.T) {
	in := SessionInput{ContextWindow: 200000, Messages: []MessageRow{
		{Type: "assistant", InputTokens: 1000, OutputTokens: 500, TokensBefore: 20000},
	}}
	score, grade := Score(in, nil)
	if score != 100 || grade != "A" {
		t.Errorf("clean session: score=%d grade=%s", score, grade)
	}
}

func TestScore_FindingsPenaltyCapped(t *testing.T) {
	in := SessionInput{ContextWindow: 200000, Messages: []MessageRow{
		{Type: "assistant", InputTokens: 1000, OutputTokens: 1000}, // billable 2000
	}}
	// Waste >> billable → penalty caps at 50.
	f := []Finding{{Kind: KindLooping, EstTokens: 999999}}
	score, _ := Score(in, f)
	if score != 50 {
		t.Errorf("want capped score 50, got %d", score)
	}
}

func TestScore_ContextFillPenalty(t *testing.T) {
	in := SessionInput{ContextWindow: 200000, Messages: []MessageRow{
		{Type: "assistant", InputTokens: 10, OutputTokens: 10, TokensBefore: 170000}, // 85% → -15
	}}
	score, _ := Score(in, nil)
	if score != 85 {
		t.Errorf("want 85 (context-fill -15), got %d", score)
	}
}

func TestGradeBands(t *testing.T) {
	cases := map[int]string{95: "A", 85: "B", 75: "C", 65: "D", 40: "F"}
	for s, want := range cases {
		if got := grade(s); got != want {
			t.Errorf("grade(%d)=%s want %s", s, got, want)
		}
	}
}
