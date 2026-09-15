package planner

import "testing"

// A live port on a shared address becomes one target per name, each carrying
// the name so the request has SNI and a Host header. Default ports drop the
// suffix; an address nothing resolves to is probed as itself.
func TestWebTargetsFanOutPerName(t *testing.T) {
	targets, dropped := webTargets("https://194.60.69.147:443", "194.60.69.147", 443,
		[]string{"telebot.lanet.ua", "mail.lanet.ua"}, 20)
	if dropped != 0 || len(targets) != 2 {
		t.Fatalf("got %d targets, %d dropped", len(targets), dropped)
	}
	if targets[0].URL != "https://telebot.lanet.ua" || targets[0].Host != "telebot.lanet.ua" ||
		targets[0].IP != "194.60.69.147" || targets[0].Port != 443 {
		t.Errorf("first target = %+v", targets[0])
	}
	if targets[1].URL != "https://mail.lanet.ua" {
		t.Errorf("second URL = %q", targets[1].URL)
	}
}

func TestWebTargetsKeepsNonDefaultPort(t *testing.T) {
	targets, _ := webTargets("http://10.0.0.1:8080", "10.0.0.1", 8080, []string{"app.example"}, 20)
	if targets[0].URL != "http://app.example:8080" {
		t.Errorf("URL = %q", targets[0].URL)
	}
}

func TestWebTargetsAddressOnlyWhenNoNames(t *testing.T) {
	targets, dropped := webTargets("http://10.0.0.1:80", "10.0.0.1", 80, nil, 20)
	if dropped != 0 || len(targets) != 1 || targets[0].Host != "" || targets[0].URL != "http://10.0.0.1:80" {
		t.Errorf("got %+v dropped=%d", targets, dropped)
	}
}

func TestWebTargetsCapCountsTheRest(t *testing.T) {
	names := []string{"a.x", "b.x", "c.x", "d.x"}
	targets, dropped := webTargets("https://1.1.1.1:443", "1.1.1.1", 443, names, 3)
	if len(targets) != 3 || dropped != 1 {
		t.Errorf("got %d targets, %d dropped", len(targets), dropped)
	}
}
