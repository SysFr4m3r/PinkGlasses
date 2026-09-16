package launch

import (
	"testing"

	"github.com/benlik386/pinkglasses/internal/domain"
)

func tg(kind, value string) domain.RunTarget { return domain.RunTarget{Kind: kind, Value: value} }

// One worker for a handful of names; one more per five; one more per /24 of a
// range; never past the Auto cap, whatever the request looks like.
func TestAutoFleetSize(t *testing.T) {
	cases := []struct {
		name    string
		targets []domain.RunTarget
		want    int
	}{
		{"one domain", []domain.RunTarget{tg("domain", "example.com")}, 1},
		{"four domains", []domain.RunTarget{tg("domain", "a"), tg("domain", "b"), tg("domain", "c"), tg("domain", "d")}, 1},
		{"five domains", []domain.RunTarget{tg("domain", "a"), tg("domain", "b"), tg("domain", "c"), tg("domain", "d"), tg("domain", "e")}, 2},
		{"a /24", []domain.RunTarget{tg("cidr", "203.0.113.0/24")}, 2},
		{"a /22 is four /24s, capped", []domain.RunTarget{tg("cidr", "10.0.0.0/22")}, 4},
		{"a /28 counts as one", []domain.RunTarget{tg("cidr", "203.0.113.0/28")}, 2},
		{"domain plus /24", []domain.RunTarget{tg("domain", "example.com"), tg("cidr", "203.0.113.0/24")}, 2},
		{"nothing", nil, 1},
	}
	for _, c := range cases {
		if got := autoFleetSize(c.targets); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}
