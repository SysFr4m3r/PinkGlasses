package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// provisionClient talks to the isolated provisioner sidecar. The api never
// touches the Docker socket itself (wiki/Architecture.md §7.3); the one thing it
// still asks for is the removal of a local worker's container when that worker
// is deleted from the fleet page. Creating workers is the scheduler's job — a
// run's fleet — and nobody's by hand.
type provisionClient struct {
	url   string
	token string
	http  *http.Client
}

func newProvisionClient() *provisionClient {
	return &provisionClient{
		url:   os.Getenv("ASM_PROVISIONER_URL"),
		token: os.Getenv("ASM_PROVISIONER_TOKEN"),
		http:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *provisionClient) enabled() bool { return p.url != "" && p.token != "" }

func (p *provisionClient) call(method, path string, body any, out any) error {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, p.url+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Provisioner-Token", p.token)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return &strErr{"provisioner: " + resp.Status + ": " + buf.String()}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// removeContainer asks the provisioner to destroy the container backing a local
// worker. Best-effort: a worker row can always be deleted even if the container
// is already gone or the provisioner is not running.
func (p *provisionClient) removeContainer(name string) error {
	if !p.enabled() || name == "" {
		return nil
	}
	return p.call(http.MethodPost, "/v1/remove", map[string]string{"name": name}, nil)
}
