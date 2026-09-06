package linkstore

import (
	"net/url"
	"path/filepath"
	"testing"
)

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(filepath.Join(dir, "links.json"))
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	return s
}

func TestDrawThenResolve(t *testing.T) {
	s := open(t, t.TempDir())

	secret, err := s.Draw("u1")
	if err != nil {
		t.Fatalf("drawing a link: %v", err)
	}

	link, ok := s.Resolve(secret)
	if !ok || link.UserID != "u1" {
		t.Fatalf("a drawn link resolves to its user, got %+v ok=%v", link, ok)
	}
}

// A secret reaches the install it was drawn for through the loopback redirect's query
// (internal/discordapi), and a query decode reads '+' as a space,
// so a drawn secret carries no character a query escapes.
func TestADrawnSecretRidesAQueryUnchanged(t *testing.T) {
	s := open(t, t.TempDir())

	secret, err := s.Draw("u1")
	if err != nil {
		t.Fatalf("drawing a link: %v", err)
	}

	if escaped := url.QueryEscape(secret); escaped != secret {
		t.Fatalf("a drawn secret rides a query unchanged, %q escapes to %q", secret, escaped)
	}
}

func TestResolveUnknownSecretAnswersFalse(t *testing.T) {
	s := open(t, t.TempDir())

	if _, ok := s.Resolve("bm90LWEtc2VjcmV0"); ok {
		t.Fatal("a secret nothing drew resolves to nobody")
	}
}

func TestTwoDrawsAreTwoLinks(t *testing.T) {
	s := open(t, t.TempDir())

	first, _ := s.Draw("u1")
	second, _ := s.Draw("u1")

	if first == second {
		t.Fatal("every draw is its own secret")
	}
	if _, ok := s.Resolve(first); !ok {
		t.Fatal("a second machine's draw leaves the first machine linked")
	}
	if _, ok := s.Resolve(second); !ok {
		t.Fatal("the second draw resolves too")
	}
}

func TestLinksSurviveAReopen(t *testing.T) {
	dir := t.TempDir()
	secret, err := open(t, dir).Draw("u1")
	if err != nil {
		t.Fatalf("drawing a link: %v", err)
	}

	link, ok := open(t, dir).Resolve(secret)
	if !ok || link.UserID != "u1" {
		t.Fatalf("a reopened store still resolves the link, got %+v ok=%v", link, ok)
	}
}

func TestDrawsBeyondTheCapEvictTheOldest(t *testing.T) {
	s := open(t, t.TempDir())

	secrets := make([]string, 0, LinksPerUser+1)
	for range LinksPerUser + 1 {
		secret, err := s.Draw("u1")
		if err != nil {
			t.Fatalf("drawing a link: %v", err)
		}
		secrets = append(secrets, secret)
	}

	if _, ok := s.Resolve(secrets[0]); ok {
		t.Fatal("a draw past the cap evicts the user's oldest link")
	}
	for _, kept := range secrets[1:] {
		if _, ok := s.Resolve(kept); !ok {
			t.Fatal("every link inside the cap stays")
		}
	}
}

func TestTheCapIsPerUser(t *testing.T) {
	s := open(t, t.TempDir())

	other, _ := s.Draw("u2")
	for range LinksPerUser {
		s.Draw("u1")
	}

	if _, ok := s.Resolve(other); !ok {
		t.Fatal("one user filling their cap evicts nothing of another's")
	}
}
