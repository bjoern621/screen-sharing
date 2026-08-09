/* The Windows export leg. See share_windows.h for what this file is and why the pool
 * exists at all. */

/* COBJMACROS is what makes d3d11.h and dxgi.h usable from C: without it the headers
 * declare C++ interfaces, and every call would have to be written through a vtable by
 * hand. It has to come before the first include of either. */
#define COBJMACROS

/* GStreamer's Direct3D 11 library is declared unstable and says so at every include
 * unless this is defined. It is defined rather than silenced elsewhere because the
 * warning is true: this file is the one place that has to be revisited when that API
 * moves, and the pin is the build's own record of where. */
#define GST_USE_UNSTABLE_API

#include <d3d11.h>
#include <dxgi.h>

#include <gst/gst.h>
#include <gst/d3d11/gstd3d11.h>

#include "share_windows.h"

/* fail writes one reason into the caller's buffer and reports failure, so every exit
 * from this file says why in the same shape. */
static int fail(char *err, int err_len, const char *format, ...) {
  va_list args;
  if (err != NULL && err_len > 0) {
    va_start(args, format);
    g_vsnprintf(err, (gulong)err_len, format, args);
    va_end(args);
  }
  return 0;
}

/* d3d11_memory_of is the sample's first memory when it is Direct3D 11 memory, and
 * NULL when it is not.
 *
 * The first memory is the whole frame here rather than one plane of it: every render
 * chain converts to RGBA before the sink, so a frame that reached this file is one
 * packed texture. A chain that stopped converting would arrive as several memories
 * and be refused here, which is the right place to notice it. */
static GstD3D11Memory *d3d11_memory_of(GstSample *sample, char *err, int err_len) {
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
  if (!gst_is_d3d11_memory(memory)) {
    /* The render chain converted somewhere this side cannot export from. It is the
     * one failure with a fix rather than a cause, so it names what it found. */
    fail(err, err_len,
         "the frames are in %s memory, and a shared texture is exported from Direct3D 11 memory",
         memory->allocator != NULL ? GST_OBJECT_NAME(memory->allocator) : "unnamed");
    return NULL;
  }
  return GST_D3D11_MEMORY_CAST(memory);
}

int screenshare_share_open(void *sample, int slots, screenshare_share_pool *pool,
                           char *err, int err_len) {
  GstD3D11Memory *memory;
  ID3D11Device *device;
  D3D11_TEXTURE2D_DESC source;
  D3D11_TEXTURE2D_DESC desc;
  int i;

  if (pool == NULL || slots <= 0 || slots > SCREENSHARE_MAX_SLOTS) {
    return fail(err, err_len, "a pool of %d slots is outside what this build allocates", slots);
  }
  memset(pool, 0, sizeof(*pool));

  memory = d3d11_memory_of((GstSample *)sample, err, err_len);
  if (memory == NULL) {
    return 0;
  }
  if (!gst_d3d11_memory_get_texture_desc(memory, &source)) {
    return fail(err, err_len, "the frame's memory carries no texture description");
  }

  /* The pool is allocated on the decoder's own device, which is what makes the
   * per-frame copy a copy inside one device rather than a trip through system memory.
   * The shell opens the handle on its own device, and DXGI is what joins the two. */
  pool->device = gst_object_ref(memory->device);
  device = gst_d3d11_device_get_device_handle(GST_D3D11_DEVICE(pool->device));

  memset(&desc, 0, sizeof(desc));
  desc.Width = source.Width;
  desc.Height = source.Height;
  desc.MipLevels = 1;
  desc.ArraySize = 1;
  desc.Format = source.Format;
  desc.SampleDesc.Count = 1;
  desc.SampleDesc.Quality = 0;
  desc.Usage = D3D11_USAGE_DEFAULT;
  /* Both bind flags, because neither side has said yet what it will do with the
   * texture: this side writes into it as a copy destination, and the shell's compositor
   * samples it. A texture missing the flag its consumer needs fails at import, which is
   * a failure one machine away from where it was configured. */
  desc.BindFlags = D3D11_BIND_SHADER_RESOURCE | D3D11_BIND_RENDER_TARGET;
  desc.CPUAccessFlags = 0;
  /* SHARED_KEYEDMUTEX rather than SHARED: the keyed mutex is the synchronization, and
   * it is also what makes GetSharedHandle's value openable in another process without
   * this backend having to reach into it. An NT handle would be the modern form and
   * would need this process to open the shell's, which two halves of one application
   * have no reason to do. */
  desc.MiscFlags = D3D11_RESOURCE_MISC_SHARED_KEYEDMUTEX;

  for (i = 0; i < slots; i++) {
    ID3D11Texture2D *texture = NULL;
    IDXGIResource *resource = NULL;
    IDXGIKeyedMutex *mutex = NULL;
    HANDLE handle = NULL;
    HRESULT hr;

    hr = ID3D11Device_CreateTexture2D(device, &desc, NULL, &texture);
    if (FAILED(hr)) {
      screenshare_share_close(pool);
      return fail(err, err_len, "creating a shared %ux%u texture failed with 0x%08lx",
                  desc.Width, desc.Height, (unsigned long)hr);
    }

    hr = ID3D11Texture2D_QueryInterface(texture, &IID_IDXGIResource, (void **)&resource);
    if (FAILED(hr)) {
      ID3D11Texture2D_Release(texture);
      screenshare_share_close(pool);
      return fail(err, err_len, "a shared texture answered no DXGI resource: 0x%08lx",
                  (unsigned long)hr);
    }
    hr = IDXGIResource_GetSharedHandle(resource, &handle);
    IDXGIResource_Release(resource);
    if (FAILED(hr)) {
      ID3D11Texture2D_Release(texture);
      screenshare_share_close(pool);
      return fail(err, err_len, "a shared texture yielded no handle: 0x%08lx",
                  (unsigned long)hr);
    }

    hr = ID3D11Texture2D_QueryInterface(texture, &IID_IDXGIKeyedMutex, (void **)&mutex);
    if (FAILED(hr)) {
      ID3D11Texture2D_Release(texture);
      screenshare_share_close(pool);
      return fail(err, err_len, "a shared texture answered no keyed mutex: 0x%08lx",
                  (unsigned long)hr);
    }

    pool->textures[i] = texture;
    pool->mutexes[i] = mutex;
    pool->handles[i] = (unsigned long long)(uintptr_t)handle;
    pool->slots = i + 1;
  }

  pool->width = desc.Width;
  pool->height = desc.Height;
  pool->format = (unsigned int)desc.Format;
  return 1;
}

int screenshare_share_blit(screenshare_share_pool *pool, int slot, void *sample,
                           char *err, int err_len) {
  GstD3D11Memory *memory;
  GstBuffer *buffer;
  GstMapInfo info;
  ID3D11DeviceContext *context;
  IDXGIKeyedMutex *mutex;
  ID3D11Resource *source;
  HRESULT hr;

  if (pool == NULL || pool->device == NULL || slot < 0 || slot >= pool->slots) {
    return fail(err, err_len, "slot %d is not one this pool lent", slot);
  }
  memory = d3d11_memory_of((GstSample *)sample, err, err_len);
  if (memory == NULL) {
    return 0;
  }
  buffer = gst_sample_get_buffer((GstSample *)sample);

  /* The map is what makes the frame current on the device. A D3D11 memory can be
   * holding its pixels in a staging texture, and reading the resource handle without
   * asking for a D3D11 mapping first would copy whatever the texture last held. */
  if (!gst_buffer_map(buffer, &info, (GstMapFlags)(GST_MAP_READ | GST_MAP_D3D11))) {
    return fail(err, err_len, "the frame could not be mapped for Direct3D 11");
  }

  source = gst_d3d11_memory_get_resource_handle(memory);
  mutex = (IDXGIKeyedMutex *)pool->mutexes[slot];

  /* Acquired outside the device lock. The wait is bounded, and holding GStreamer's
   * device while waiting on a consumer would stall every element on that device
   * behind a window that is busy. */
  hr = IDXGIKeyedMutex_AcquireSync(mutex, SCREENSHARE_PRODUCER_KEY,
                                   SCREENSHARE_ACQUIRE_TIMEOUT_MS);
  if (hr != S_OK) {
    gst_buffer_unmap(buffer, &info);
    return fail(err, err_len, "slot %d was not handed back within %d ms: 0x%08lx", slot,
                SCREENSHARE_ACQUIRE_TIMEOUT_MS, (unsigned long)hr);
  }

  gst_d3d11_device_lock(GST_D3D11_DEVICE(pool->device));
  context = gst_d3d11_device_get_device_context_handle(GST_D3D11_DEVICE(pool->device));
  ID3D11DeviceContext_CopySubresourceRegion(
      context, (ID3D11Resource *)pool->textures[slot], 0, 0, 0, 0, source,
      gst_d3d11_memory_get_subresource_index(memory), NULL);
  /* Flushed rather than left queued: the release below tells the shell the slot is
   * readable, and a copy still sitting in this device's command queue would be a
   * promise about pixels that have not been written. */
  ID3D11DeviceContext_Flush(context);
  gst_d3d11_device_unlock(GST_D3D11_DEVICE(pool->device));

  IDXGIKeyedMutex_ReleaseSync(mutex, SCREENSHARE_CONSUMER_KEY);
  gst_buffer_unmap(buffer, &info);
  return 1;
}

void screenshare_share_close(screenshare_share_pool *pool) {
  int i;

  if (pool == NULL) {
    return;
  }
  for (i = 0; i < pool->slots; i++) {
    if (pool->mutexes[i] != NULL) {
      IDXGIKeyedMutex_Release((IDXGIKeyedMutex *)pool->mutexes[i]);
      pool->mutexes[i] = NULL;
    }
    if (pool->textures[i] != NULL) {
      ID3D11Texture2D_Release((ID3D11Texture2D *)pool->textures[i]);
      pool->textures[i] = NULL;
    }
    pool->handles[i] = 0;
  }
  pool->slots = 0;
  if (pool->device != NULL) {
    gst_object_unref(pool->device);
    pool->device = NULL;
  }
}
