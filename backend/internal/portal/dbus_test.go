package portal

import (
	"sync"
	"testing"
)

// A token repeated across two Opens puts both of them on one Request object path, so the counter is
// exercised from several goroutines at once.
// The race detector is what catches the unguarded increment; the uniqueness check catches the lost
// update it produces.
func TestConcurrentTokensAreDistinct(t *testing.T) {
	const callers = 64

	tokens := make([]string, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := range tokens {
		go func() {
			defer wg.Done()
			tokens[i] = newToken()
		}()
	}
	wg.Wait()

	seen := make(map[string]bool, callers)
	for _, token := range tokens {
		if seen[token] {
			t.Fatalf("%s was handed out twice", token)
		}
		seen[token] = true
	}
}
