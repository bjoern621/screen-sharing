package discordrpc

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDiscord answers a client the way a desktop Discord does, and keeps what it was sent.
type fakeDiscord struct {
	mu       sync.Mutex
	commands []setActivity
	// answer is the frame every command is answered with, an accepted command by default.
	answerOp      uint32
	answerPayload []byte
}

// serve reads frames until the connection closes, answering each.
func (f *fakeDiscord) serve(conn net.Conn) {
	for {
		op, payload, err := readFrame(conn)
		if err != nil {
			return
		}

		f.mu.Lock()
		if op == opFrame {
			var command setActivity
			if err := json.Unmarshal(payload, &command); err == nil {
				f.commands = append(f.commands, command)
			}
		}
		answerOp, answer := f.answerOp, f.answerPayload
		f.mu.Unlock()

		if err := writeFrame(conn, answerOp, answer); err != nil {
			return
		}
	}
}

// stated is the activities the fake was sent, in order.
func (f *fakeDiscord) stated() []*activityMessage {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]*activityMessage, 0, len(f.commands))
	for _, command := range f.commands {
		out = append(out, command.Args.Activity)
	}
	return out
}

// connected is a client talking to a fake, the handshake already done.
func connected(t *testing.T) (*Client, *fakeDiscord) {
	t.Helper()

	here, there := net.Pipe()
	fake := &fakeDiscord{answerOp: opFrame, answerPayload: []byte(`{"cmd":"DISPATCH","evt":"READY"}`)}
	go fake.serve(there)

	client := &Client{conn: here}
	if err := client.handshake("an-application"); err != nil {
		t.Fatalf("handshaking: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
		there.Close()
	})
	return client, fake
}

func TestAFrameRoundTrips(t *testing.T) {
	var buffer bytes.Buffer
	payload := []byte(`{"cmd":"SET_ACTIVITY"}`)

	if err := writeFrame(&buffer, opFrame, payload); err != nil {
		t.Fatalf("writing: %v", err)
	}

	op, read, err := readFrame(&buffer)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if op != opFrame {
		t.Errorf("opcode %d, sent %d", op, opFrame)
	}
	if !bytes.Equal(read, payload) {
		t.Errorf("payload %q, sent %q", read, payload)
	}
}

func TestTheHandshakeNamesTheApplication(t *testing.T) {
	here, there := net.Pipe()
	defer there.Close()

	// Read off the wire rather than off a fake's command list, which holds SET_ACTIVITY alone.
	read := make(chan []byte, 1)
	go func() {
		op, payload, err := readFrame(there)
		if err != nil {
			return
		}
		if op == opHandshake {
			read <- payload
		}
		writeFrame(there, opFrame, []byte(`{"evt":"READY"}`))
	}()

	client := &Client{conn: here}
	if err := client.handshake("1234567890"); err != nil {
		t.Fatalf("handshaking: %v", err)
	}
	client.Close()

	select {
	case payload := <-read:
		if !strings.Contains(string(payload), `"client_id":"1234567890"`) {
			t.Errorf("the handshake carries the application id, sent %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("the handshake sent no frame carrying the application")
	}
}

func TestOneActivityIsStatedOnce(t *testing.T) {
	client, fake := connected(t)
	activity := Activity{Details: "Sharing a screen", State: "General", Readers: 1, Members: 4}

	for range 3 {
		if err := client.SetActivity(activity); err != nil {
			t.Fatalf("stating: %v", err)
		}
	}

	if got := len(fake.stated()); got != 1 {
		t.Errorf("%d activities stated, and the three calls name one state", got)
	}
}

func TestAChangedActivityIsStatedAgain(t *testing.T) {
	client, fake := connected(t)

	if err := client.SetActivity(Activity{Details: "Sharing a screen", Readers: 1, Members: 4}); err != nil {
		t.Fatalf("stating: %v", err)
	}
	if err := client.SetActivity(Activity{Details: "Sharing a screen", Readers: 2, Members: 4}); err != nil {
		t.Fatalf("stating again: %v", err)
	}

	stated := fake.stated()
	if len(stated) != 2 {
		t.Fatalf("%d activities stated, and the two calls name two states", len(stated))
	}
	if stated[1].Party.Size != [2]int{2, 4} {
		t.Errorf("the party is %v, and the second call names two readers of four members", stated[1].Party.Size)
	}
}

func TestClearingTakesTheActivityOff(t *testing.T) {
	client, fake := connected(t)

	if err := client.SetActivity(Activity{Details: "Sharing a screen"}); err != nil {
		t.Fatalf("stating: %v", err)
	}
	if err := client.ClearActivity(); err != nil {
		t.Fatalf("clearing: %v", err)
	}

	stated := fake.stated()
	if len(stated) != 2 {
		t.Fatalf("%d activities stated, and a clear is a state of its own", len(stated))
	}
	if stated[1] != nil {
		t.Errorf("a clear carries no activity, carried %+v", stated[1])
	}
}

func TestAPartyOfNobodyIsLeftOff(t *testing.T) {
	client, fake := connected(t)

	if err := client.SetActivity(Activity{Details: "Sharing a screen"}); err != nil {
		t.Fatalf("stating: %v", err)
	}

	if party := fake.stated()[0].Party; party != nil {
		t.Errorf("a party of nobody is left off the activity, carried %v", party.Size)
	}
}

func TestAnActivityIsWatchedRatherThanStreamed(t *testing.T) {
	client, fake := connected(t)

	if err := client.SetActivity(Activity{Details: "Sharing a screen"}); err != nil {
		t.Fatalf("stating: %v", err)
	}

	// Type 1 needs a Twitch or YouTube address, which no activity stated here carries.
	if got := fake.stated()[0].Type; got != activityWatching {
		t.Errorf("activity type %d, and this states %d", got, activityWatching)
	}
}

func TestARefusedActivityIsReported(t *testing.T) {
	here, there := net.Pipe()
	fake := &fakeDiscord{
		answerOp:      opFrame,
		answerPayload: []byte(`{"evt":"ERROR","data":{"code":4000,"message":"Invalid Client ID"}}`),
	}
	go fake.serve(there)
	defer there.Close()

	client := &Client{conn: here}
	err := client.SetActivity(Activity{Details: "Sharing a screen"})
	client.Close()

	if err == nil {
		t.Fatal("a refused activity is an error")
	}
	if !strings.Contains(err.Error(), "Invalid Client ID") {
		t.Errorf("the refusal carries the client's own words, said %v", err)
	}
}

func TestAClosedConnectionIsReported(t *testing.T) {
	here, there := net.Pipe()
	fake := &fakeDiscord{
		answerOp:      opClose,
		answerPayload: []byte(`{"code":4000,"message":"Invalid Client ID"}`),
	}
	go fake.serve(there)
	defer there.Close()

	client := &Client{conn: here}
	err := client.handshake("nobody")
	client.Close()

	if err == nil {
		t.Fatal("a client hanging up on the handshake is an error")
	}
	if !strings.Contains(err.Error(), "hung up") {
		t.Errorf("the error names the hang-up, said %v", err)
	}
}

func TestTheWindowBoundsSends(t *testing.T) {
	var w window
	at := time.Now()

	for i := range sendsPerWindow {
		if !w.take(at) {
			t.Fatalf("send %d of %d fits the window", i+1, sendsPerWindow)
		}
	}
	if w.take(at) {
		t.Errorf("a %dth send inside %s is past what Discord allows", sendsPerWindow+1, sendWindow)
	}
	if !w.take(at.Add(sendWindow)) {
		t.Errorf("a send after the window has run out fits a fresh one")
	}
}

func TestAStatedActivityOutlastsASpentWindow(t *testing.T) {
	client, fake := connected(t)

	// Every state differs, so each one asks for a send and the window is what stops them.
	for i := range sendsPerWindow + 3 {
		if err := client.SetActivity(Activity{Details: "Sharing a screen", Readers: i, Members: 9}); err != nil {
			t.Fatalf("stating: %v", err)
		}
	}

	if got := len(fake.stated()); got != sendsPerWindow {
		t.Errorf("%d activities stated, and the window allows %d", got, sendsPerWindow)
	}
}
