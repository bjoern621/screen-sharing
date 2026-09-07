// Package avatars holds the pictures this app draws people with, read from Discord's CDN.
//
// A shell draws bytes and never an address: the backend is the side that reaches the network,
// and a picture is a fact like every other on the contract (docs/ipc-api.md).
// What is held is content and not state: an address names one picture forever,
// so a hit is the same answer a read would give.
//
// Every read is an Umgebungsfehler, and a row missing its picture is still the row.
package avatars

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/discordavatar"
)

// timeout bounds one read. Nothing waits on it, so it is generous rather than tight.
const timeout = 10 * time.Second

// maxBytes bounds what one read admits.
// A 64-pixel PNG is single-digit kilobytes, so this is a bound on a service gone wrong.
const maxBytes = 1 << 20

// maxHeld bounds the pictures kept at once, one per person this install has seen in a channel.
const maxHeld = 512

// retryAfter is how long a read that failed is left alone.
// The poll asks again every couple of seconds, and a CDN that is down stays down for longer than that.
const retryAfter = 5 * time.Minute

// Cache answers pictures by address and reads the ones it lacks.
// Safe for concurrent use.
type Cache struct {
	// landed fires after a read lands, the pictures a shell holds being whatever the last event carried.
	landed func()
	client *http.Client
	now    func() time.Time
	// host is the one host an address may name, discordavatar.Host outside a test.
	host string

	mu       sync.Mutex
	held     map[string][]byte
	reading  map[string]struct{}
	deferred map[string]time.Time
}

// New is an empty cache, calling landed after every read that lands.
func New(landed func()) *Cache {
	assert.IsNotNil(landed, "a cache announces the pictures it reads")

	return &Cache{
		landed:   landed,
		client:   &http.Client{Timeout: timeout},
		now:      time.Now,
		host:     discordavatar.Host,
		held:     map[string][]byte{},
		reading:  map[string]struct{}{},
		deferred: map[string]time.Time{},
	}
}

// Bytes is the picture at address, empty until one is read.
//
// Idempotent: an address already held answers from what was read,
// an address being read answers empty and starts nothing second,
// and a first ask starts the read that a later ask answers from.
func (c *Cache) Bytes(address string) []byte {
	if address == "" {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if held, ok := c.held[address]; ok {
		return held
	}
	if _, reading := c.reading[address]; reading {
		return nil
	}
	if until, deferred := c.deferred[address]; deferred && c.now().Before(until) {
		return nil
	}
	if !c.addressed(address) {
		return nil
	}

	c.reading[address] = struct{}{}
	go c.read(address)
	return nil
}

// addressed reports an address this cache reads, and says why where it does not.
//
// Discord's CDN and nothing else: the address comes from the manager, which is another machine,
// and a fetch of whatever it names would make this app that machine's client.
// Caller holds mu.
func (c *Cache) addressed(address string) bool {
	at, err := url.Parse(address)
	if err != nil || at.Host != c.host {
		logger.Warnf("a picture at %s is not read, pictures coming from %s alone", address, c.host)
		c.deferred[address] = c.now().Add(retryAfter)
		return false
	}
	return true
}

// read fetches one picture and holds what it answered, or defers the address it did not.
func (c *Cache) read(address string) {
	picture, err := c.fetch(address)
	if err != nil {
		logger.Warnf("a picture at %s is not read, so that row carries none: %v", address, err)
	}

	c.mu.Lock()
	delete(c.reading, address)
	if err != nil {
		c.deferred[address] = c.now().Add(retryAfter)
		c.mu.Unlock()
		return
	}
	c.admit(address, picture)
	c.mu.Unlock()

	c.landed()
}

// admit holds one picture, dropping another where the cache is full.
// Which one goes costs a read the next time that address is asked for. Caller holds mu.
func (c *Cache) admit(address string, picture []byte) {
	for len(c.held) >= maxHeld {
		for other := range c.held {
			delete(c.held, other)
			break
		}
	}
	c.held[address] = picture
}

// fetch is one read of the CDN.
func (c *Cache) fetch(address string) ([]byte, error) {
	resp, err := c.client.Get(address)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("the CDN answered %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}
