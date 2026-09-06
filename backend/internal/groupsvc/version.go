package groupsvc

import (
	"net/http"

	"bjoernblessin.de/go-utils/util/assert"
)

// naming writes the product and version every answer carries.
//
// The header rather than a route of its own, so one request tells a caller both that the service
// answered and what is answering: a relay check dials the key route and reads this off it
// (internal/reach), and the relay beside this one names itself the same way.
// Around the whole mux, a route that refuses being as much of an answer as one that serves.
func naming(version string, next http.Handler) http.Handler {
	assert.Assert(version != "", "a service names the version it is serving from")
	assert.IsNotNil(next, "a header is written around a handler")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "groupd/"+version)
		next.ServeHTTP(w, r)
	})
}
