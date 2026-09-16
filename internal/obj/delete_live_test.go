package obj

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/benlik386/pinkglasses/internal/config"
)

// Against a real store (MinIO in the compose stack): put an object through a
// presigned URL, delete it, and confirm it is gone. Runs only when
// ASM_S3_ENDPOINT is set, so CI without a store skips it.
func TestDeleteLive(t *testing.T) {
	endpoint := os.Getenv("ASM_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("ASM_S3_ENDPOINT not set")
	}
	st := New(config.S3{
		Endpoint: endpoint, Bucket: os.Getenv("ASM_S3_BUCKET"),
		AccessKey: os.Getenv("ASM_S3_ACCESS_KEY"), SecretKey: os.Getenv("ASM_S3_SECRET_KEY"),
	})
	key := "screenshots/test-delete/" + time.Now().Format("150405") + ".png"
	ctx := context.Background()

	put, err := st.PresignPut(key, time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, put, strings.NewReader("not really a png"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode >= 300 {
		t.Fatalf("put: %v %v", err, resp)
	}
	resp.Body.Close()

	if err := st.Delete(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	get, _ := st.PresignGet(key, time.Minute, time.Now())
	resp, err = http.Get(get)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete, GET = %s, want 404", resp.Status)
	}
	// Deleting again is not an error: cleanup is idempotent.
	if err := st.Delete(ctx, key); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}
