package readylive_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Bo0mer/readylive"
)

// TODO(ivan): Add tests for the provided server options.

func TestServer(t *testing.T) {
	srv := &http.Server{
		Handler: http.NotFoundHandler(),
		Addr:    "localhost:53218",
	}

	wsrv := readylive.WrapServer(srv,
		readylive.WaitBeforeShutdown(1*time.Second))

	go func() {
		err := wsrv.ListenAndServe()
		if err != http.ErrServerClosed {
			t.Error(err)
		}
	}()

	time.Sleep(time.Millisecond)

	checkStatus(t, fmt.Sprintf("http://%s/health", srv.Addr), 200)
	checkStatus(t, fmt.Sprintf("http://%s/ready", srv.Addr), 200)

	go func() {
		time.Sleep(time.Millisecond)

		checkStatus(t, fmt.Sprintf("http://%s/health", srv.Addr), 200)
		checkStatus(t, fmt.Sprintf("http://%s/ready", srv.Addr), 503)
	}()
	err := wsrv.Shutdown(context.Background())
	if err != nil {
		t.Errorf("want nil error, got %v", err)
	}
}

func checkStatus(t *testing.T, url string, status int) {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != status {
		t.Errorf("want status code %d, got %d", status, resp.StatusCode)
	}
}
