package session

import (
	"maps"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// Chains is what the decode backend offers to render with, which is what a picker
// draws. It is asked on every call rather than kept: which chains this machine can
// run is the backend's answer off its element registry, and a copy here would be a
// second version of it.
func (s *Session) Chains() []player.Chain { return s.chains() }

// DefaultRenderChain is the chain every stream that chose nothing renders through:
// the one the window picked, or the backend's own default while nobody has picked.
//
// The backend marks that default on the offer itself (player.Chain.Default), so this
// side reads it rather than inferring it from the order the chains are offered in: a
// list reordered for a picker's sake would otherwise move the default with it.
func (s *Session) DefaultRenderChain() string {
	if s.renderChain != "" {
		return s.renderChain
	}
	offered := s.chains()
	assert.Assert(len(offered) > 0, "a backend offers a render chain to draw with")

	for _, c := range offered {
		if c.Default {
			return c.Name
		}
	}
	assert.Never("an offered render chain is the backend's default", len(offered))
	return ""
}

// RenderOverride is the chain stream i renders through on its own account, and ""
// while it follows the window's default.
//
// It is what a stream's picker shows rather than RenderChain: a stream that follows
// the default has to read as following it, so that moving the default moves it,
// instead of reading as having chosen the chain the default happens to be on.
func (s *Session) RenderOverride(i int) string { return s.renderChains[s.at(i).stream.Name] }

// RenderChain is the chain stream i actually renders through: its own override, or
// the window's default. It is what start opens the stream's player on.
func (s *Session) RenderChain(i int) string {
	if own := s.RenderOverride(i); own != "" {
		return own
	}
	return s.DefaultRenderChain()
}

// SetRenderChain pins stream i to one render chain, or hands it back to the
// window's default with "".
//
// A watched stream restarts on the new chain. A chain is fixed when the pipeline is
// parsed, so there is no moving a running one, which is the same reason a stream
// whose watch leg moved restarts rather than follows (SetRoster).
//
// Asking for the pin the stream already holds does nothing: the same chain twice
// must not cost a restart.
func (s *Session) SetRenderChain(i int, name string) {
	assert.Assert(name == "" || s.offers(name),
		"a chosen render chain is one the backend offers and this build can run", name)

	e := s.at(i)
	if s.renderChains[e.stream.Name] == name {
		return
	}
	was := s.RenderChain(i)
	if name == "" {
		delete(s.renderChains, e.stream.Name)
		logger.Infof("%q renders through the window's default chain again", e.stream.Name)
	} else {
		s.renderChains[e.stream.Name] = name
		logger.Infof("%q renders through the %q chain", e.stream.Name, name)
	}
	s.notify(Change{Kind: RenderChanged, Index: i})
	s.persistRender.Schedule()

	// The restart goes last, and reads the chain back through RenderChain rather than
	// being handed it: what the model holds now is what the player has to open on.
	// A pin that names the chain the default was already on moves no pipeline.
	if s.RenderChain(i) != was && s.at(i).state.Watched() {
		s.Restart(i)
	}
}

// SetDefaultRenderChain moves the chain every stream without one of its own renders
// through, and restarts the watched streams that were on it.
//
// A stream with a pin of its own is left alone: it chose against the default, so the
// default moving is not about it.
func (s *Session) SetDefaultRenderChain(name string) {
	assert.Assert(s.offers(name),
		"a chosen render chain is one the backend offers and this build can run", name)

	if s.renderChain == name {
		return
	}
	s.renderChain = name
	logger.Infof("the window renders through the %q chain", name)
	s.notify(Change{Kind: RenderChanged, Index: noStream})
	s.persistRender.Schedule()

	for i := range s.entries {
		if s.at(i).state.Watched() && s.RenderOverride(i) == "" {
			s.Restart(i)
		}
	}
}

// offers reports whether the backend offers that chain and this machine can run it,
// which is what a chosen one is held to.
func (s *Session) offers(name string) bool {
	for _, c := range s.chains() {
		if c.Name == name {
			return c.Available
		}
	}
	return false
}

// writeRender records the render chains in force, so the next run draws through
// them. It is called from the coalescer after every change, like the arrangement's
// own write; a lost write costs the last change and nothing more.
//
// The whole map goes out, entries of streams this run never saw included: they are
// the choices of a machine that is away, and the run it comes back in is the one
// that needs them.
func (s *Session) writeRender() {
	r := layout.Render{Chain: s.renderChain, Streams: maps.Clone(s.renderChains)}
	s.store.SaveRender(r)
	logger.Debugf("render chains written: default %q, %d streams with one of their own", r.Chain, len(r.Streams))
}

// rememberedRender is the render choice the state file offers, with the chain names
// this build cannot place taken out.
//
// The file sits in a config directory where anything can edit it, and a chain name
// is a build's vocabulary rather than a fact about the world: an older or newer
// binary, or a hand-edited file, can name a chain this one has never heard of. That
// is a condition to survive rather than a bug on this side, so the name is dropped
// with a warning and what it stood for falls back to the default, the way
// rememberedOrder drops a slot it cannot rank.
//
// A chain this build knows but this machine cannot run is kept: which chains
// register is the machine's business, and the backend says so and falls back on its
// own when a stream opens on one.
func rememberedRender(r layout.Render, offered []player.Chain) (string, map[string]string) {
	known := func(name string) bool {
		for _, c := range offered {
			if c.Name == name {
				return true
			}
		}
		return false
	}

	chain := r.Chain
	if chain != "" && !known(chain) {
		logger.Warnf("the remembered render chain %q is not one this build offers, rendering through the default", chain)
		chain = ""
	}
	streams := make(map[string]string, len(r.Streams))
	for name, c := range r.Streams {
		if name == "" {
			logger.Warnf("a remembered render chain names no stream, ignoring that entry")
			continue
		}
		if !known(c) {
			logger.Warnf("%q is remembered on the render chain %q, which is not one this build offers, rendering it through the default", name, c)
			continue
		}
		streams[name] = c
	}
	return chain, streams
}
