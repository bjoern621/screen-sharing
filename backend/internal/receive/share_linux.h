/*
 * The Linux export leg of the frame channel, in C because that is the language the two
 * APIs it joins are written in: GStreamer's OpenGL library, which owns the decoded
 * frame, and EGL, which is what turns a texture into a descriptor another process can
 * open (docs/viewer-architecture.md, "The frame channel").
 *
 * Nothing here maps a frame. A decoded texture is copied on the GPU into a texture this
 * file allocated, and it is that one the shell imports; the pixels never enter system
 * memory and never enter a message.
 *
 * The pool is the unit of ownership. Its textures live for as long as one subscription
 * does, which is what makes a consumer that died a consumer whose memory is reclaimed
 * rather than one holding the decoder's own pool hostage.
 *
 * A descriptor is not a number another process can use, so the handles this side ends up
 * with do not travel in a message: the Go half lends them over a socket, and what crosses
 * the contract is where that socket is.
 */

#ifndef SCREENSHARE_SHARE_LINUX_H
#define SCREENSHARE_SHARE_LINUX_H

#include <gst/gst.h>

/* How many slots a pool may hold. The caller asks for fewer; this is the bound the
 * fixed-size arrays below are written against, and asking for more is a caller that
 * changed slotCount without changing this. */
#define SCREENSHARE_MAX_SLOTS 8

/* What a driver answers when it has no name for the layout it just exported.
 *
 * It is DRM_FORMAT_MOD_INVALID, and it means the frames carry whatever tiling the driver
 * picked rather than a layout either side can spell. Both halves are the same GPU here,
 * so an import that states no modifier resolves it the same way the export did - which is
 * the descriptor's own convention and not a guess this code makes. */
#define SCREENSHARE_MODIFIER_IMPLICIT 0x00ffffffffffffffULL

/* screenshare_share_pool is one subscription's lent memory.
 *
 * The pointers are void* rather than their real types so that the Go side can hold this
 * struct without cgo having to parse the GL headers behind them. */
typedef struct {
  void *context; /* GstGLContext *, referenced for the pool's lifetime */
  void *slots[SCREENSHARE_MAX_SLOTS];  /* GstGLMemory *, the textures frames are copied into */
  int fds[SCREENSHARE_MAX_SLOTS];      /* one dmabuf descriptor per slot */
  unsigned int strides[SCREENSHARE_MAX_SLOTS];
  unsigned long long offsets[SCREENSHARE_MAX_SLOTS];
  int slot_count;
  unsigned int width;
  unsigned int height;
  /* fourcc is the DRM format the driver exported, which is the layout the shell has to
   * import with; modifier is the tiling that goes with it. */
  unsigned int fourcc;
  unsigned long long modifier;
} screenshare_share_pool;

/* screenshare_share_open allocates a pool for frames like the one in sample.
 *
 * It fails, with the reason written into err, where the sample's frames are not in GL
 * memory or the GL context is not an EGL one. Both are a render chain or a display server
 * that cannot export rather than a fault, so the caller turns them into a sentence: the
 * fix is a different chain, and not a retry.
 *
 * Returns 1 on success and 0 on failure. */
int screenshare_share_open(void *sample, int slots, screenshare_share_pool *pool, char *err,
                           int err_len);

/* screenshare_share_blit copies one frame into one slot.
 *
 * The copy is a GPU-side blit between two textures: no map, no download, no format
 * conversion. It exists because the decoder's own texture is part of a pool this side does
 * not own, and handing its descriptor over would tie the decoder's ability to allocate to a
 * consumer's willingness to release.
 *
 * It returns once the copy has finished on the device rather than once it has been
 * submitted. That is this leg's whole synchronization: the contract carries no fence for a
 * descriptor handle, so a frame is announced after it is there instead of alongside a
 * promise that it will be.
 *
 * Returns 1 on success and 0 on failure, and a failure is one frame rather than the
 * stream: the caller drops it and offers the next one. */
int screenshare_share_blit(screenshare_share_pool *pool, int slot, void *sample, char *err,
                           int err_len);

/* screenshare_share_close frees the pool and closes its descriptors.
 *
 * Every descriptor the consumer received is its own, so what a close ends is this side's
 * reference: the memory goes when the last one does. It is safe on a zeroed pool, which is
 * what makes a failed open safe to close. */
void screenshare_share_close(screenshare_share_pool *pool);

#endif /* SCREENSHARE_SHARE_LINUX_H */
