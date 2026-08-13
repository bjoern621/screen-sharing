/*
 * The Windows export leg of the frame channel, in C because that is the language the
 * two APIs it joins are written in: GStreamer's Direct3D 11 library, which owns the
 * decoded frame, and DXGI, which is what turns a texture into a name another process
 * can open (docs/viewer-architecture.md, "The frame channel").
 *
 * Nothing here maps a frame. A decoded texture is copied on the GPU into a texture
 * this file allocated, and it is that one the shell imports; the pixels never enter
 * system memory and never enter a message.
 *
 * The pool is the unit of ownership. Its textures live for as long as one
 * subscription does, which is what makes a consumer that died a consumer whose memory
 * is reclaimed rather than one holding the decoder's own pool hostage.
 */

#ifndef SCREENSHARE_SHARE_WINDOWS_H
#define SCREENSHARE_SHARE_WINDOWS_H

#include <gst/gst.h>

/* How many slots a pool may hold. The caller asks for fewer; this is the bound the
 * fixed-size arrays below are written against, and asking for more is a caller that
 * changed slotCount without changing this. */
#define SCREENSHARE_MAX_SLOTS 8

/* How long a keyed-mutex acquire waits before it is treated as a consumer that will
 * never release. A frame is 16 ms at 60 Hz, so a second is three orders of magnitude
 * of slack: reaching it means the other side is gone rather than slow, and the frame
 * is dropped instead of blocking the decode thread for the rest of the run. */
#define SCREENSHARE_ACQUIRE_TIMEOUT_MS 1000

/* The keyed-mutex protocol. Each side acquires with its own key and releases with the
 * other's, so a slot alternates between the two and neither can write while the other
 * reads. Zero is the key a freshly created keyed mutex is unlocked with, which is what
 * makes the producer the side that goes first. */
#define SCREENSHARE_PRODUCER_KEY 0
#define SCREENSHARE_CONSUMER_KEY 1

/* screenshare_share_pool is one subscription's lent memory.
 *
 * The pointers are void* rather than their real types so that the Go side can hold
 * this struct without cgo having to parse d3d11.h, which it cannot: the header is C++
 * unless COBJMACROS is defined, and cgo compiles the preamble on its own terms. */
typedef struct {
  void *device;  /* GstD3D11Device *, referenced for the pool's lifetime */
  void *textures[SCREENSHARE_MAX_SLOTS];
  void *mutexes[SCREENSHARE_MAX_SLOTS];
  unsigned long long handles[SCREENSHARE_MAX_SLOTS];
  int slots;
  unsigned int width;
  unsigned int height;
  /* format is the DXGI format the slots carry, which is the one the decoded frames
   * were in: a pool that converted would be a second converter behind the chain's. */
  unsigned int format;
} screenshare_share_pool;

/* screenshare_share_open allocates a pool for frames like the one in sample.
 *
 * It fails, with the reason written into err, where the sample's memory is not
 * Direct3D 11 memory. That is a render chain that converted somewhere else rather
 * than a fault: the caller turns it into a sentence naming the chain, because the fix
 * is to pick a different one and not to try again.
 *
 * Returns 1 on success and 0 on failure. */
int screenshare_share_open(void *sample, int slots, screenshare_share_pool *pool,
                           char *err, int err_len);

/* screenshare_share_blit copies one frame into one slot, between an acquire and a
 * release of that slot's keyed mutex.
 *
 * The copy is a GPU-side CopySubresourceRegion: no map, no download, no format
 * conversion. It exists because the decoder's own texture is part of a pool this side
 * does not own, and handing its handle over would tie the decoder's ability to
 * allocate to a consumer's willingness to release.
 *
 * Returns 1 on success and 0 on failure, and a failure is one frame rather than the
 * stream: the caller drops it and offers the next one. */
int screenshare_share_blit(screenshare_share_pool *pool, int slot, void *sample,
                           char *err, int err_len);

/* screenshare_share_close releases everything the pool holds. Every handle handed to
 * a consumer names nothing afterwards, which is why this runs when a subscription
 * ends and never while one lives. Safe to call on a zeroed pool and safe to call
 * twice. */
void screenshare_share_close(screenshare_share_pool *pool);

#endif /* SCREENSHARE_SHARE_WINDOWS_H */
