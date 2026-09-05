package groupsvc

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Reports keeps the bundles members send in,
// and answers the name each was stored under (internal/reportstore).
//
// An interface for the reason Streams is one: the route is tested without a disk.
// nil is a deployment keeping none, and the route says so.
type Reports interface {
	Save(body io.Reader) (id string, err error)
}

// ReportsPerHour is how many reports one address may send in an hour.
//
// Sending is open for the reason creation is:
// a report is most needed where the settings hold no working group,
// so a key would lock out exactly the caller the route exists for.
// Behind a reverse proxy the address is the proxy's,
// which makes this a backstop the way CreationsPerHour is.
const ReportsPerHour = 30

// reportBodyLimit is how much of a report is read, bytes.
// The app builds bundles of a few tail-capped logs well under this (internal/report),
// so anything past it is not one.
const reportBodyLimit = 16 << 20

// takeReport stores one report bundle and answers the name it landed under.
// The doors are the hourly bound and the body limit, and no key.
func (s *Service) takeReport(w http.ResponseWriter, r *http.Request) {
	if s.reports == nil {
		refuse(w, http.StatusNotFound, "this relay keeps no reports")
		return
	}
	if !s.allowReport(caller(r)) {
		refuse(w, http.StatusTooManyRequests, "too many reports sent from here in the last hour")
		return
	}

	id, err := s.reports.Save(http.MaxBytesReader(w, r.Body, reportBodyLimit))
	if err != nil {
		var over *http.MaxBytesError
		if errors.As(err, &over) {
			refuse(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("a report is at most %d MiB, and this one is larger", reportBodyLimit>>20))
			return
		}
		refuse(w, http.StatusInternalServerError, "the report could not be stored")
		return
	}
	answer(w, map[string]string{"reportId": id})
}

// allowReport reports whether this caller may send another report,
// and records the send where they may.
func (s *Service) allowReport(caller string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return recordWithin(s.sent, caller, ReportsPerHour, s.now())
}
