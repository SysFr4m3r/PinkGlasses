package domain

import (
	"testing"
	"time"
)

func TestMergeTargetsUnionsAuthorization(t *testing.T) {
	who, when := "admin", time.Now()
	rows := []ScopeTarget{
		{Kind: "domain", Value: "a.example", Mode: ModePassiveOnly},
		{Kind: "domain", Value: "b.example", Mode: ModeActive, AuthorizedBy: &who, AuthorizedAt: &when},
		{Kind: "domain", Value: "a.example", Mode: ModeActive, AuthorizedBy: &who, AuthorizedAt: &when}, // same value, another group
		{Kind: "domain", Value: "b.example", Mode: ModeExclude},                                         // excluded in one group
		{Kind: "domain", Value: "c.example", Mode: ModePassiveOnly},
		{Kind: "domain", Value: "c.example", Mode: ModePassiveOnly},
	}
	got := MergeTargets(rows)
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	if got[0].Value != "a.example" || !got[0].Authorized() {
		t.Errorf("a.example should be authorized through its second group: %+v", got[0])
	}
	if got[1].Value != "b.example" || got[1].Mode != ModeExclude {
		t.Errorf("b.example excluded anywhere should stay excluded: %+v", got[1])
	}
	if got[2].Value != "c.example" || got[2].Authorized() {
		t.Errorf("c.example passive in both stays passive: %+v", got[2])
	}
}
