/* The Linux export leg. See share_linux.h for what this file is and why the pool exists
 * at all. */

#include <stdarg.h>
#include <string.h>
#include <unistd.h>

#include <EGL/egl.h>
#include <EGL/eglext.h>

#include <gst/gl/egl/gsteglimage.h>
#include <gst/gl/gl.h>
#include <gst/gl/gstglfuncs.h>
#include <gst/gst.h>

#include "share_linux.h"

/* The one texture layout every slot is allocated in. The render chain converts to RGBA
 * before the sink, so a slot matches the frames rather than converting a second time. */
#define SCREENSHARE_SLOT_TARGET GST_GL_TEXTURE_TARGET_2D
#define SCREENSHARE_SLOT_FORMAT GST_GL_RGBA

/* fail writes one reason into the caller's buffer and reports failure, so every exit from
 * this file says why in the same shape. */
static int fail(char *err, int err_len, const char *format, ...) {
  va_list args;
  if (err != NULL && err_len > 0) {
    va_start(args, format);
    g_vsnprintf(err, (gulong)err_len, format, args);
    va_end(args);
  }
  return 0;
}

/* job is what crosses onto the GL thread and what comes back from it.
 *
 * Every GL and EGL call in this file runs on the thread that holds the context current,
 * because that is the only thread the textures exist on. gst_gl_context_thread_add is the
 * crossing, and it carries one pointer, so the arguments and the answer travel in this. */
typedef struct {
  screenshare_share_pool *pool;
  GstGLMemory *source;
  int slots;
  int slot;
  char *err;
  int err_len;
  int ok;
} job;

/* gl_memory_of is the sample's first memory when it is GL memory, and NULL when it is not.
 *
 * The first memory is the whole frame here rather than one plane of it: every render chain
 * converts to RGBA before the sink, so a frame that reached this file is one packed
 * texture. A chain that stopped converting would arrive as several memories and be refused
 * here, which is the right place to notice it. */
static GstGLMemory *gl_memory_of(GstSample *sample, char *err, int err_len) {
  GstBuffer *buffer;
  GstMemory *memory;

  if (sample == NULL) {
    fail(err, err_len, "the sink handed over no sample");
    return NULL;
  }
  buffer = gst_sample_get_buffer(sample);
  if (buffer == NULL) {
    fail(err, err_len, "the sample carries no buffer");
    return NULL;
  }
  if (gst_buffer_n_memory(buffer) != 1) {
    fail(err, err_len, "the frame arrived in %u memories, and a converted frame is one",
         gst_buffer_n_memory(buffer));
    return NULL;
  }
  memory = gst_buffer_peek_memory(buffer, 0);
  if (!gst_is_gl_memory(memory)) {
    /* The render chain converted somewhere this side cannot export from. It is the one
     * failure with a fix rather than a cause, so it names what it found. */
    fail(err, err_len, "the frames are in %s memory, and a dmabuf is exported from GL memory",
         memory->allocator != NULL ? GST_OBJECT_NAME(memory->allocator) : "unnamed");
    return NULL;
  }
  return (GstGLMemory *)memory;
}

/* export_slot turns one slot's texture into a descriptor, with the layout that goes with
 * it. It runs on the GL thread with the context current.
 *
 * The EGLImage is a name for the texture and not a second allocation, and it is dropped as
 * soon as the descriptor exists: what keeps the memory alive afterwards is the descriptor
 * on this side and the one the consumer receives, either of which is enough. */
static int export_slot(GstGLContext *context, screenshare_share_pool *pool, int slot,
                       char *err, int err_len) {
  PFNEGLEXPORTDMABUFIMAGEQUERYMESAPROC query;
  PFNEGLEXPORTDMABUFIMAGEMESAPROC export;
  GstEGLImage *image;
  EGLDisplay display;
  EGLuint64KHR modifier = 0;
  EGLint fourcc = 0;
  EGLint planes = 0;
  int fds[4] = {-1, -1, -1, -1};
  EGLint strides[4] = {0, 0, 0, 0};
  EGLint offsets[4] = {0, 0, 0, 0};

  display = eglGetCurrentDisplay();
  if (display == EGL_NO_DISPLAY) {
    return fail(err, err_len, "the GL context holds no EGL display to export through");
  }

  query = (PFNEGLEXPORTDMABUFIMAGEQUERYMESAPROC)eglGetProcAddress(
      "eglExportDMABUFImageQueryMESA");
  export = (PFNEGLEXPORTDMABUFIMAGEMESAPROC)eglGetProcAddress("eglExportDMABUFImageMESA");
  if (query == NULL || export == NULL) {
    return fail(err, err_len,
                "this driver's EGL does not carry EGL_MESA_image_dma_buf_export, so a "
                "decoded frame cannot be named to another process");
  }

  image = gst_egl_image_from_texture(context, (GstGLMemory *)pool->slots[slot], NULL);
  if (image == NULL) {
    return fail(err, err_len, "slot %d's texture yielded no EGL image", slot);
  }

  if (!query(display, gst_egl_image_get_image(image), &fourcc, &planes, &modifier)) {
    gst_egl_image_unref(image);
    return fail(err, err_len, "slot %d's layout could not be read: EGL error 0x%04x", slot,
                eglGetError());
  }
  if (planes != 1) {
    /* A converted RGBA frame is one plane. Several would mean the slot was allocated in
     * something this file did not ask for, and the contract lends one descriptor per
     * slot. */
    gst_egl_image_unref(image);
    return fail(err, err_len, "slot %d exported %d planes, and a converted frame is one",
                slot, planes);
  }

  if (!export(display, gst_egl_image_get_image(image), fds, strides, offsets)) {
    gst_egl_image_unref(image);
    return fail(err, err_len, "slot %d could not be exported: EGL error 0x%04x", slot,
                eglGetError());
  }
  gst_egl_image_unref(image);

  pool->fds[slot] = fds[0];
  pool->strides[slot] = (unsigned int)strides[0];
  pool->offsets[slot] = (unsigned long long)offsets[0];

  /* Every slot is the same allocation made the same way, so the format and the tiling
   * belong to the pool. Two slots disagreeing would mean the driver answered differently
   * for identical requests, and a pool announced with one layout and holding another draws
   * as noise. */
  if (slot == 0) {
    pool->fourcc = (unsigned int)fourcc;
    pool->modifier = (unsigned long long)modifier;
    return 1;
  }
  if (pool->fourcc != (unsigned int)fourcc ||
      pool->modifier != (unsigned long long)modifier) {
    return fail(err, err_len, "slot %d exported a layout the rest of the pool does not carry",
                slot);
  }
  return 1;
}

/* open_on_gl allocates the slots and exports them. */
static void open_on_gl(GstGLContext *context, job *work) {
  screenshare_share_pool *pool = work->pool;
  GstAllocator *allocator;
  GstVideoInfo info;
  int i;

  allocator = gst_allocator_find(GST_GL_MEMORY_ALLOCATOR_NAME);
  if (allocator == NULL) {
    fail(work->err, work->err_len, "this GStreamer registers no GL memory allocator");
    return;
  }
  gst_video_info_set_format(&info, GST_VIDEO_FORMAT_RGBA, pool->width, pool->height);

  for (i = 0; i < work->slots; i++) {
    GstGLVideoAllocationParams *params;
    GstGLMemory *slot;

    params = gst_gl_video_allocation_params_new(context, NULL, &info, 0, NULL,
                                                SCREENSHARE_SLOT_TARGET, SCREENSHARE_SLOT_FORMAT);
    slot = (GstGLMemory *)gst_gl_base_memory_alloc(GST_GL_BASE_MEMORY_ALLOCATOR(allocator),
                                                   (GstGLAllocationParams *)params);
    gst_gl_allocation_params_free((GstGLAllocationParams *)params);
    if (slot == NULL) {
      gst_object_unref(allocator);
      fail(work->err, work->err_len, "a %ux%u slot texture could not be allocated", pool->width,
           pool->height);
      return;
    }

    pool->slots[i] = slot;
    pool->slot_count = i + 1;
    if (!export_slot(context, pool, i, work->err, work->err_len)) {
      gst_object_unref(allocator);
      return;
    }
  }

  gst_object_unref(allocator);
  work->ok = 1;
}

/* blit_on_gl copies one frame into one slot and waits for the device to finish it. */
static void blit_on_gl(GstGLContext *context, job *work) {
  screenshare_share_pool *pool = work->pool;
  GstGLMemory *slot = (GstGLMemory *)pool->slots[work->slot];

  if (!gst_gl_memory_copy_into(work->source, slot->tex_id, SCREENSHARE_SLOT_TARGET,
                               SCREENSHARE_SLOT_FORMAT, (gint)pool->width,
                               (gint)pool->height)) {
    fail(work->err, work->err_len, "the frame could not be copied into slot %d", work->slot);
    return;
  }

  /* Waited for rather than flushed. The consumer is told the slot holds a frame as soon as
   * this returns, and it imports a descriptor rather than a texture, so there is nothing in
   * its own command stream for the driver to order this copy against. */
  context->gl_vtable->Finish();
  work->ok = 1;
}

/* close_on_gl frees the slot textures. The descriptors are closed by the caller, which can
 * do it from any thread. */
static void close_on_gl(GstGLContext *context, job *work) {
  screenshare_share_pool *pool = work->pool;
  int i;

  (void)context;
  for (i = 0; i < pool->slot_count; i++) {
    if (pool->slots[i] != NULL) {
      gst_memory_unref(GST_MEMORY_CAST(pool->slots[i]));
      pool->slots[i] = NULL;
    }
  }
  work->ok = 1;
}

int screenshare_share_open(void *sample, int slots, screenshare_share_pool *pool, char *err,
                           int err_len) {
  GstGLMemory *memory;
  GstGLContext *context;
  job work;
  int i;

  if (pool == NULL || slots <= 0 || slots > SCREENSHARE_MAX_SLOTS) {
    return fail(err, err_len, "a pool of %d slots is outside what this build allocates", slots);
  }
  memset(pool, 0, sizeof(*pool));
  /* A descriptor of zero is standard input rather than an empty slot, so the empty value
   * is written rather than left as the zeroing's. Every exit from a half-built pool runs
   * the same close, and it closes what it holds. */
  for (i = 0; i < SCREENSHARE_MAX_SLOTS; i++) {
    pool->fds[i] = -1;
  }

  memory = gl_memory_of((GstSample *)sample, err, err_len);
  if (memory == NULL) {
    return 0;
  }
  context = GST_GL_BASE_MEMORY_CAST(memory)->context;
  if ((gst_gl_context_get_gl_platform(context) & GST_GL_PLATFORM_EGL) == 0) {
    /* GLX has no export of its own, so the frames are on the GPU and unnameable. It is a
     * display server and a GStreamer platform rather than a chain, and it is refused for
     * the same reason a chain that downloads is: the fix is upstream of this call. */
    return fail(err, err_len,
                "the frames were decoded on a %s context, and a dmabuf is exported from an "
                "EGL one",
                gst_gl_platform_to_string(gst_gl_context_get_gl_platform(context)));
  }

  /* The pool is allocated on the decoder's own context, which is what makes the per-frame
   * copy a copy inside one context rather than a trip through system memory. The shell
   * imports the descriptor on its own device, and the kernel is what joins the two. */
  pool->context = gst_object_ref(context);
  pool->width = GST_VIDEO_INFO_WIDTH(&memory->info);
  pool->height = GST_VIDEO_INFO_HEIGHT(&memory->info);

  work.pool = pool;
  work.source = memory;
  work.slots = slots;
  work.slot = 0;
  work.err = err;
  work.err_len = err_len;
  work.ok = 0;
  gst_gl_context_thread_add(context, (GstGLContextThreadFunc)open_on_gl, &work);
  if (!work.ok) {
    screenshare_share_close(pool);
    return 0;
  }
  return 1;
}

int screenshare_share_blit(screenshare_share_pool *pool, int slot, void *sample, char *err,
                           int err_len) {
  GstGLMemory *memory;
  job work;

  if (pool == NULL || pool->context == NULL || slot < 0 || slot >= pool->slot_count) {
    return fail(err, err_len, "slot %d is not one this pool lent", slot);
  }
  memory = gl_memory_of((GstSample *)sample, err, err_len);
  if (memory == NULL) {
    return 0;
  }
  if (GST_GL_BASE_MEMORY_CAST(memory)->context != (GstGLContext *)pool->context) {
    /* One pipeline holds one GL context for its whole run, so this is a frame from a
     * decode the pool was not opened on rather than a renegotiation. Copying across two
     * contexts would read a texture name that means something else in each. */
    return fail(err, err_len, "the frame was decoded on a GL context this pool was not opened on");
  }

  work.pool = pool;
  work.source = memory;
  work.slots = pool->slot_count;
  work.slot = slot;
  work.err = err;
  work.err_len = err_len;
  work.ok = 0;
  gst_gl_context_thread_add((GstGLContext *)pool->context, (GstGLContextThreadFunc)blit_on_gl,
                            &work);
  return work.ok;
}

void screenshare_share_close(screenshare_share_pool *pool) {
  int i;

  if (pool == NULL) {
    return;
  }
  if (pool->context != NULL) {
    job work;
    work.pool = pool;
    work.source = NULL;
    work.slots = pool->slot_count;
    work.slot = 0;
    work.err = NULL;
    work.err_len = 0;
    work.ok = 0;
    gst_gl_context_thread_add((GstGLContext *)pool->context,
                              (GstGLContextThreadFunc)close_on_gl, &work);
    gst_object_unref(pool->context);
    pool->context = NULL;
  }

  for (i = 0; i < pool->slot_count; i++) {
    if (pool->fds[i] >= 0) {
      close(pool->fds[i]);
      pool->fds[i] = -1;
    }
    pool->strides[i] = 0;
    pool->offsets[i] = 0;
  }
  pool->slot_count = 0;
}
