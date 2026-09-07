package avatars

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// serve is a CDN stand-in the cache is pointed at, and the count of reads it took.
func serve(t *testing.T, body string) (*Cache, *atomic.Int64, chan struct{}) {
	t.Helper()

	var reads atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	landed := make(chan struct{}, 8)
	c := New(func() { landed <- struct{}{} })
	at, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("reading the stand-in's address: %v", err)
	}
	c.host = at.Host
	return c, &reads, landed
}

func waitForLanding(t *testing.T, landed chan struct{}) {
	t.Helper()
	select {
	case <-landed:
	case <-time.After(2 * time.Second):
		t.Fatal("the picture never landed")
	}
}

func TestAPictureIsEmptyUntilItLands(t *testing.T) {
	c, _, landed := serve(t, "png-bytes")
	address := "http://" + c.host + "/avatars/u1/h1.png"

	if got := c.Bytes(address); len(got) != 0 {
		t.Fatalf("a first ask answered %q, want nothing until a read lands", got)
	}

	waitForLanding(t, landed)
	if got := string(c.Bytes(address)); got != "png-bytes" {
		t.Fatalf("the landed picture is %q, want what the CDN answered", got)
	}
}

func TestAPictureIsReadOnce(t *testing.T) {
	c, reads, landed := serve(t, "png-bytes")
	address := "http://" + c.host + "/avatars/u1/h1.png"

	c.Bytes(address)
	waitForLanding(t, landed)
	for range 5 {
		c.Bytes(address)
	}

	if got := reads.Load(); got != 1 {
		t.Fatalf("the address was read %d times, want the one read a picture takes", got)
	}
}

func TestAnAddressOffDiscordsCdnIsNeverRead(t *testing.T) {
	c, reads, _ := serve(t, "png-bytes")

	if got := c.Bytes("https://elsewhere.example/avatars/u1/h1.png"); len(got) != 0 {
		t.Fatalf("an address off the CDN answered %q, want nothing", got)
	}

	time.Sleep(50 * time.Millisecond)
	if got := reads.Load(); got != 0 {
		t.Fatalf("the stand-in was read %d times, want none for an address off the CDN", got)
	}
}

func TestNoAddressIsNoPicture(t *testing.T) {
	c, reads, _ := serve(t, "png-bytes")

	if got := c.Bytes(""); len(got) != 0 {
		t.Fatalf("an empty address answered %q, want nothing", got)
	}
	if got := reads.Load(); got != 0 {
		t.Fatalf("the stand-in was read %d times, want none for an empty address", got)
	}
}
