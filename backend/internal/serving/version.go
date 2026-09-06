// Package serving holds what the HTTP services beside the relay answer alike.
package serving

import (
	"net/http"

	"bjoernblessin.de/go-utils/util/assert"
)

// Naming writes the product and version every answer carries, "groupd/0.9.0".
//
// The header rather than a route of its own, so one request tells a caller both that the service
// answered and what is answering: a relay check reads this off whatever it dialled
// (internal/reach), and MediaMTX names itself the same way on the legs beside these.
// Around the whole mux, a route that refuses being as much of an answer as one that serves.
func Naming(product, version string, next http.Handler) http.Handler {
	assert.Assert(product != "", "a service names the product answering")
	assert.Assert(version != "", "a service names the version it is serving from")
	assert.IsNotNil(next, "a header is written around a handler")

	name := product + "/" + version
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", name)
		next.ServeHTTP(w, r)
	})
}
