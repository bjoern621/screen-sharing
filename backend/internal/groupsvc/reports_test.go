package groupsvc

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/membership"
	"bjoernblessin.de/screenshare/internal/token"
)

// stored keeps what arrived in memory, standing in for the disk.
type stored struct {
	bodies [][]byte
}

func (s *stored) Save(body io.Reader) (string, error) {
	read, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("reading a report: %w", err)
	}
	s.bodies = append(s.bodies, read)
	return fmt.Sprintf("report-%d", len(s.bodies)), nil
}

// reporting is a service keeping reports in memory.
func reporting(t *testing.T) (*Service, *stored) {
	t.Helper()
	signer, err := token.NewSigner()
	if err != nil {
		t.Fatalf("drawing a signing key: %v", err)
	}
	reports := &stored{}
	return New(signer, nil, membership.New(&carrying{}), &keyed{}, reports), reports
}

// A report is taken without a group key:
// it is most needed where the settings hold no working group.
func TestAReportIsStoredAndItsNameAnswered(t *testing.T) {
	s, reports := reporting(t)

	status, body := call(t, s, "POST", "/reports", "bundle")
	if status != http.StatusOK {
		t.Fatalf("sending a report answered %d: %v", status, body)
	}
	if body["reportId"] != "report-1" {
		t.Fatalf("a report's answer names where it landed, got %v", body)
	}
	if len(reports.bodies) != 1 || string(reports.bodies[0]) != "bundle" {
		t.Fatalf("a report is stored as it arrived, got %q", reports.bodies)
	}
}

// nil is a deployment keeping none, and a caller is told so rather than answered a name
// nothing stands behind.
func TestADeploymentKeepingNoReportsRefuses(t *testing.T) {
	s := service(t)

	status, _ := call(t, s, "POST", "/reports", "bundle")
	if status != http.StatusNotFound {
		t.Fatalf("a deployment keeping no reports answered %d", status)
	}
}

// The body bound refuses a bundle larger than any this app builds,
// and nothing is stored.
func TestAReportPastTheBodyBoundIsRefused(t *testing.T) {
	s, reports := reporting(t)

	status, _ := call(t, s, "POST", "/reports", strings.Repeat("x", reportBodyLimit+1))
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("an oversized report answered %d", status)
	}
	if len(reports.bodies) != 0 {
		t.Fatalf("an oversized report is not stored, %d were", len(reports.bodies))
	}
}

// One address sends ReportsPerHour and the next one is refused,
// the bound CreationsPerHour puts on creation applied here.
func TestReportsFromOneAddressAreBounded(t *testing.T) {
	s, _ := reporting(t)

	for i := 0; i < ReportsPerHour; i++ {
		status, body := call(t, s, "POST", "/reports", "bundle")
		if status != http.StatusOK {
			t.Fatalf("report %d answered %d: %v", i+1, status, body)
		}
	}

	status, _ := call(t, s, "POST", "/reports", "bundle")
	if status != http.StatusTooManyRequests {
		t.Fatalf("a report over the hourly bound answered %d", status)
	}
}
