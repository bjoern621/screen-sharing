using System.Runtime.InteropServices;
using Avalonia;
using Avalonia.Controls;
using Avalonia.OpenGL;
using Avalonia.OpenGL.Controls;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using static Avalonia.OpenGL.GlConsts;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// The surface for a handle the compositor does not import: a dmabuf descriptor, imported through EGL by this
/// control and drawn as a texture.
///
/// <b>Why this one draws where the other one hands over.</b> Avalonia's compositor imports a shared texture
/// and an opaque descriptor, and it does not import a dmabuf; so on this handle type the import is EGL's -
/// <c>eglCreateImageKHR(EGL_LINUX_DMA_BUF_EXT)</c> and <c>glEGLImageTargetTexture2DOES</c> - and what reaches
/// the compositor is what this control drew rather than the lent memory itself
/// (<c>docs/viewer-architecture.md</c>, "The frame channel").
///
/// <b>It is still a visual among visuals.</b> <see cref="OpenGlControlBase"/> renders into a composition
/// surface rather than into a window of its own, so a figure or a menu over a tile stays over it - which is
/// the rule a native child window would break (<c>avalonia/README.md</c>).
///
/// <b>Nothing here reads a pixel.</b> The descriptor names memory the backend decoded into, the driver
/// samples it where it lies, and no frame enters system memory or a message.
/// What this process spends is one draw of one quad per frame.
///
/// <b>The renderer has to be an EGL one.</b> A GLX context has no import for a descriptor, so a tile on such
/// a window says so instead of drawing.
/// Both halves of the app steer their GL towards EGL for this reason, which is a requirement of the handle
/// type and not a preference: the backend's is <c>GST_GL_PLATFORM</c> and this side's is
/// <c>X11RenderingMode</c> (<c>Program.cs</c>).
/// </summary>
internal sealed class DmaBufSurface : OpenGlControlBase, ITileSurface
{
    /// <summary>
    /// One pool, as this surface needs it: the layout to import with and the descriptors to import.
    /// It is a record so a pass can tell what it has imported from what it was given by identity alone, which
    /// is what makes the import happen once per pool.
    /// </summary>
    private sealed record Lent(
        int Width,
        int Height,
        int Fourcc,
        ulong Modifier,
        bool TopLeftOrigin,
        int[] Descriptors,
        ulong[] Offsets,
        uint[] Strides);

    /// <summary>What the next pass is to import, and what the last one did. They differ for one pass.</summary>
    private Lent? _lent;
    private Lent? _imported;

    /// <summary>
    /// The imported slots, by index.
    /// Each descriptor is imported once and drawn from many times, which is the whole point of the pool: a
    /// per-frame import would be a per-frame trip through the driver.
    /// </summary>
    private readonly List<int> _textures = [];
    private readonly List<IntPtr> _images = [];

    /// <summary>The slot the next pass draws, and none before the first frame.</summary>
    private uint? _slot;

    /// <summary>
    /// What the caller is waiting for.
    /// One pass answers every ask made before it ran, because what a caller wants to know is that the surface
    /// has drawn since it asked, and one draw satisfies any number of asks.
    /// </summary>
    private TaskCompletionSource<string?>? _pass;

    /// <summary>Why this surface is not drawing, and null while it is.</summary>
    private string? _failure;

    private int _program;
    private int _vertices;
    private int _vertexArray;
    private int _cornerAttribute;
    private int _frameUniform;
    private int _flipUniform;

    private IntPtr _display;
    private CreateImage? _createImage;
    private DestroyImage? _destroyImage;

    public Control View => this;

    public FrameHandleType Handle => FrameHandleType.DmabufFd;

    public async Task<string?> ImportAsync(FramePool pool, CancellationToken cancellation)
    {
        // The descriptors are read before the pass rather than inside it: it is a socket round trip with
        // another process, and a render pass is not a place to wait on one.
        var descriptors = await FrameDescriptors
            .ReceiveAsync(pool.FdSocket, pool.Slots.Count, cancellation)
            .ConfigureAwait(true);

        ReleaseUnimported();
        _lent = new Lent(
            (int)pool.Width,
            (int)pool.Height,
            FourccOf(pool.Format),
            pool.Modifier,
            pool.TopLeftOrigin,
            descriptors,
            [.. pool.Slots.Select(slot => slot.Planes.Count > 0 ? slot.Planes[0].Offset : 0)],
            [.. pool.Slots.Select(slot => slot.Planes.Count > 0 ? slot.Planes[0].Stride : 0)]);
        _slot = null;

        return await PassAsync(cancellation).ConfigureAwait(true);
    }

    public Task DrawAsync(uint slot, CancellationToken cancellation)
    {
        _slot = slot;
        return PassAsync(cancellation);
    }

    /// <summary>
    /// Asks for a pass and waits for it.
    ///
    /// The wait is what the loan is made of: a slot is free once the draw that read it has finished on the
    /// device, and this completes there.
    ///
    /// It is bounded, for the one case that would otherwise be silent.
    /// A renderer that fails to come up at all never calls a pass, and an unbounded wait there is a tile that
    /// holds a slot and says nothing for the rest of the run.
    /// Past the bound the surface answers for itself: the slot goes back and the tile shows why it is empty.
    /// </summary>
    private async Task<string?> PassAsync(CancellationToken cancellation)
    {
        _pass ??= new TaskCompletionSource<string?>(TaskCreationOptions.RunContinuationsAsynchronously);
        RequestNextFrameRendering();

        try
        {
            return await _pass.Task.WaitAsync(PassBound, cancellation).ConfigureAwait(true);
        }
        catch (TimeoutException)
        {
            _failure = "This window's renderer did not open an OpenGL surface for the frames.";
            return _failure;
        }
    }

    /// <summary>
    /// How long a pass is waited for.
    /// Long enough that a renderer building its first context and shaders is not mistaken for one that
    /// failed, and short enough that a tile which will never draw says so while the reader is still looking
    /// at it.
    /// </summary>
    private static readonly TimeSpan PassBound = TimeSpan.FromSeconds(5);

    protected override void OnOpenGlInit(GlInterface gl)
    {
        // Read while the context is current, which is the only time there is one to read.
        try
        {
            _display = eglGetCurrentDisplay();
        }
        catch (DllNotFoundException)
        {
            _display = IntPtr.Zero;
        }

        _createImage = Resolve<CreateImage>(gl, "eglCreateImageKHR");
        _destroyImage = Resolve<DestroyImage>(gl, "eglDestroyImageKHR");

        Build(gl);
    }

    protected override void OnOpenGlDeinit(GlInterface gl)
    {
        Drop(gl);

        if (_program != 0)
        {
            gl.DeleteProgram(_program);
            _program = 0;
        }
        if (_vertices != 0)
        {
            gl.DeleteBuffer(_vertices);
            _vertices = 0;
        }
        if (_vertexArray != 0)
        {
            gl.DeleteVertexArray(_vertexArray);
            _vertexArray = 0;
        }

        Forget();

        // A caller waiting on a pass that will not come now: a tile is told rather than left holding a slot
        // it can neither draw nor release.
        _failure = "This tile's renderer went away.";
        Answer();
    }

    /// <summary>
    /// The context is gone and so is everything that was imported on it.
    ///
    /// Nothing is freed here, because freeing a texture takes the context that owned it.
    /// What is dropped is this side's record of them, so no pass draws from a name that means nothing; the
    /// descriptors are closed and the pool is forgotten, since re-importing one takes descriptors that have
    /// already been read.
    /// </summary>
    protected override void OnOpenGlLost()
    {
        _textures.Clear();
        _images.Clear();
        _slot = null;
        Forget();

        _failure = "This window's renderer was lost, so this tile is waiting for the next pool.";
        Answer();
    }

    /// <summary>
    /// Drops the pool this surface holds, closing what was never imported.
    ///
    /// The clearing is the half that matters: a surface that kept the pool would skip the import when it was
    /// handed the same one again, and it has nothing to import it from - a descriptor is closed as soon as it
    /// has been read.
    /// </summary>
    private void Forget()
    {
        ReleaseUnimported();
        _lent = null;
        _imported = null;
    }

    /// <summary>
    /// Closes the descriptors of a pool that arrived and was never imported, which is what the import would
    /// otherwise have done.
    /// Two pools inside one render pass is the case: a stream that renegotiated twice while the window was
    /// busy.
    /// </summary>
    private void ReleaseUnimported()
    {
        if (_lent is not null && !ReferenceEquals(_lent, _imported))
        {
            FrameDescriptors.Release(_lent.Descriptors);
        }
    }

    /// <summary>
    /// The one render function: it imports what it has been lent, draws the slot it was given, and answers
    /// whoever asked for the pass.
    ///
    /// Every pass writes the whole surface.
    /// There is no branch that leaves the last frame on screen, so a pool with nothing drawable in it clears
    /// to black rather than showing a picture from a pool that is gone.
    /// </summary>
    protected override void OnOpenGlRender(GlInterface gl, int fb)
    {
        Import(gl);
        Paint(gl);

        // Finished rather than flushed: the slot goes back to the backend as soon as this pass answers, and a
        // draw still queued would be a read of memory the next frame is being written into.
        gl.Finish();
        Answer();
    }

    /// <summary>
    /// Imports the pool this surface has been lent, and does nothing when the one it holds is already that
    /// pool.
    /// </summary>
    private void Import(GlInterface gl)
    {
        if (ReferenceEquals(_lent, _imported))
        {
            return;
        }

        Drop(gl);
        var lent = _lent;
        _imported = lent;
        if (lent is null)
        {
            return;
        }

        try
        {
            if (_display == IntPtr.Zero || _createImage is null)
            {
                _failure = "This window's renderer cannot open the kind of shared frame this machine decodes into.";
                return;
            }

            for (var slot = 0; slot < lent.Descriptors.Length; slot++)
            {
                if (!Import(gl, lent, slot))
                {
                    Drop(gl);
                    _failure = "The frames this machine decoded could not be opened by this window's renderer.";
                    return;
                }
            }
            _failure = null;
        }
        finally
        {
            // The descriptors are this process's own and EGL holds its own reference to what they name, so
            // they are closed as soon as the import has read them - successful or not.
            // What keeps the memory alive afterwards is the image.
            FrameDescriptors.Release(lent.Descriptors);
        }
    }

    /// <summary>One slot: a descriptor, the layout it is read with, and the texture it becomes.</summary>
    private unsafe bool Import(GlInterface gl, Lent lent, int slot)
    {
        var attributes = stackalloc int[19];
        var i = 0;
        attributes[i++] = EGL_WIDTH;
        attributes[i++] = lent.Width;
        attributes[i++] = EGL_HEIGHT;
        attributes[i++] = lent.Height;
        attributes[i++] = EGL_LINUX_DRM_FOURCC_EXT;
        attributes[i++] = lent.Fourcc;
        attributes[i++] = EGL_DMA_BUF_PLANE0_FD_EXT;
        attributes[i++] = lent.Descriptors[slot];
        attributes[i++] = EGL_DMA_BUF_PLANE0_OFFSET_EXT;
        attributes[i++] = (int)lent.Offsets[slot];
        attributes[i++] = EGL_DMA_BUF_PLANE0_PITCH_EXT;
        attributes[i++] = (int)lent.Strides[slot];
        if (lent.Modifier != ImplicitModifier)
        {
            // Stated only where the driver named one.
            // An implicit modifier means the exporter let the driver pick the layout, and an import that then
            // states a modifier is stating a layout nobody chose.
            attributes[i++] = EGL_DMA_BUF_PLANE0_MODIFIER_LO_EXT;
            attributes[i++] = (int)(lent.Modifier & 0xffffffff);
            attributes[i++] = EGL_DMA_BUF_PLANE0_MODIFIER_HI_EXT;
            attributes[i++] = (int)(lent.Modifier >> 32);
        }
        attributes[i] = EGL_NONE;

        var image = _createImage!(_display, IntPtr.Zero, EGL_LINUX_DMA_BUF_EXT, IntPtr.Zero, attributes);
        if (image == IntPtr.Zero)
        {
            return false;
        }

        var texture = gl.GenTexture();
        gl.BindTexture(GL_TEXTURE_2D, texture);
        gl.EGLImageTargetTexture2DOES(GL_TEXTURE_2D, image);
        if (gl.GetError() != GL_NO_ERROR)
        {
            gl.DeleteTexture(texture);
            _destroyImage?.Invoke(_display, image);
            return false;
        }

        // Linear and clamped, because a tile draws the frame at whatever size it was arranged at: the rungs
        // the render size is rounded onto mean the picture is usually a little larger than the box it goes
        // in.
        gl.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
        gl.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
        gl.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
        gl.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
        gl.BindTexture(GL_TEXTURE_2D, 0);

        _textures.Add(texture);
        _images.Add(image);
        return true;
    }

    /// <summary>Draws the slot this surface was given, over a cleared frame.</summary>
    private unsafe void Paint(GlInterface gl)
    {
        var scaling = TopLevel.GetTopLevel(this)?.RenderScaling ?? 1;
        gl.Viewport(0, 0, Math.Max(1, (int)(Bounds.Width * scaling)),
            Math.Max(1, (int)(Bounds.Height * scaling)));
        gl.ClearColor(0, 0, 0, 1);
        gl.Clear(GL_COLOR_BUFFER_BIT);

        if (_program == 0 || _imported is null || _slot is not { } slot || slot >= _textures.Count)
        {
            return;
        }

        gl.UseProgram(_program);
        gl.ActiveTexture(GL_TEXTURE0);
        gl.BindTexture(GL_TEXTURE_2D, _textures[(int)slot]);
        gl.Uniform1i(_frameUniform, 0);
        // The flip is read off what the pool stated rather than assumed: a texture holds its first row where
        // OpenGL puts the bottom of the picture, and a backend whose frames start at the top is the case this
        // uniform exists for.
        gl.Uniform1f(_flipUniform, _imported.TopLeftOrigin ? 1 : 0);

        gl.BindVertexArray(_vertexArray);
        gl.BindBuffer(GL_ARRAY_BUFFER, _vertices);
        gl.EnableVertexAttribArray(_cornerAttribute);
        gl.VertexAttribPointer(_cornerAttribute, 2, GL_FLOAT, 0, 2 * sizeof(float), IntPtr.Zero);
        gl.DrawArrays(GL_TRIANGLE_STRIP, 0, new IntPtr(4));

        gl.BindBuffer(GL_ARRAY_BUFFER, 0);
        gl.BindVertexArray(0);
    }

    /// <summary>Answers the pass that was asked for, with what this surface can and cannot do.</summary>
    private void Answer()
    {
        var pass = _pass;
        _pass = null;
        pass?.TrySetResult(_failure);
    }

    /// <summary>Drops the imports of the pool that is no longer current.</summary>
    private void Drop(GlInterface gl)
    {
        foreach (var texture in _textures)
        {
            gl.DeleteTexture(texture);
        }
        _textures.Clear();

        foreach (var image in _images)
        {
            _destroyImage?.Invoke(_display, image);
        }
        _images.Clear();

        _slot = null;
    }

    /// <summary>
    /// Builds the one program and the one quad this surface draws with.
    ///
    /// A shader rather than a blit: <c>glBlitFramebuffer</c> is OpenGL ES 3 and this control runs on whatever
    /// context the window's renderer made, and a blit cannot flip a picture whose first row is its top
    /// either.
    /// </summary>
    private unsafe void Build(GlInterface gl)
    {
        var vertex = gl.CreateShader(GL_VERTEX_SHADER);
        var vertexError = gl.CompileShaderAndGetError(vertex, Shader(VertexShader, fragment: false));
        var fragment = gl.CreateShader(GL_FRAGMENT_SHADER);
        var fragmentError = gl.CompileShaderAndGetError(fragment, Shader(FragmentShader, fragment: true));
        if (vertexError is not null || fragmentError is not null)
        {
            gl.DeleteShader(vertex);
            gl.DeleteShader(fragment);
            _failure = "This window's renderer could not build the shader a tile draws with.";
            return;
        }

        var program = gl.CreateProgram();
        gl.AttachShader(program, vertex);
        gl.AttachShader(program, fragment);
        var linkError = gl.LinkProgramAndGetError(program);
        gl.DeleteShader(vertex);
        gl.DeleteShader(fragment);
        if (linkError is not null)
        {
            gl.DeleteProgram(program);
            _failure = "This window's renderer could not link the shader a tile draws with.";
            return;
        }

        _program = program;
        _cornerAttribute = gl.GetAttribLocationString(program, "aCorner");
        _frameUniform = gl.GetUniformLocationString(program, "uFrame");
        _flipUniform = gl.GetUniformLocationString(program, "uFlip");

        _vertices = gl.GenBuffer();
        _vertexArray = gl.GenVertexArray();
        gl.BindVertexArray(_vertexArray);
        gl.BindBuffer(GL_ARRAY_BUFFER, _vertices);
        var corners = stackalloc float[8] { -1, -1, 1, -1, -1, 1, 1, 1 };
        gl.BufferData(GL_ARRAY_BUFFER, new IntPtr(8 * sizeof(float)), (IntPtr)corners, GL_STATIC_DRAW);
        gl.BindBuffer(GL_ARRAY_BUFFER, 0);
        gl.BindVertexArray(0);
    }

    /// <summary>
    /// One shader in the dialect the context speaks.
    /// The two differ in three keywords and in the name of the texture lookup, and the source is written once
    /// in the older of them.
    /// </summary>
    private string Shader(string source, bool fragment)
    {
        var version = GlVersion.Type == GlProfileType.OpenGL
            ? OperatingSystem.IsMacOS() ? 150 : 120
            : 100;

        var head = "#version " + version + "\n";
        if (GlVersion.Type == GlProfileType.OpenGLES)
        {
            head += "precision mediump float;\n";
        }

        if (version >= 150)
        {
            source = source
                .Replace("attribute", "in")
                .Replace("texture2D(", "texture(")
                .Replace("varying", fragment ? "in" : "out");
            if (fragment)
            {
                source = source
                    .Replace("//DECLAREGLFRAG", "out vec4 outFragColor;")
                    .Replace("gl_FragColor", "outFragColor");
            }
        }

        return head + source;
    }

    private const string VertexShader = """
        attribute vec2 aCorner;
        uniform float uFlip;
        varying vec2 vFrame;
        void main()
        {
            vec2 corner = aCorner * 0.5 + 0.5;
            vFrame = vec2(corner.x, mix(corner.y, 1.0 - corner.y, uFlip));
            gl_Position = vec4(aCorner, 0.0, 1.0);
        }
        """;

    private const string FragmentShader = """
        uniform sampler2D uFrame;
        varying vec2 vFrame;
        //DECLAREGLFRAG
        void main()
        {
            gl_FragColor = texture2D(uFrame, vFrame);
        }
        """;

    public ValueTask DisposeAsync()
    {
        // The GL objects go with the control's own deinit, which is the only place there is a context to free
        // them on.
        // What is left here is the wait: a tile that dropped this surface is not going to ask again, and a
        // pass nobody answers would outlive it.
        Forget();
        _slot = null;
        Answer();
        return ValueTask.CompletedTask;
    }

    /// <summary>The DRM format a slot is imported with, which is what the pool's format stands for on this handle kind.</summary>
    private static int FourccOf(FrameFormat format) => format switch
    {
        // A fourcc names the order the channels sit in memory, which is the reverse of the order it is
        // spelled in: DRM_FORMAT_ABGR8888 is a red byte first.
        FrameFormat.B8G8R8A8Unorm => 0x34325241, // AR24
        _ => 0x34324241,                         // AB24
    };

    private static T? Resolve<T>(GlInterface gl, string name) where T : Delegate
    {
        var address = gl.GetProcAddress(name);
        return address == IntPtr.Zero ? null : Marshal.GetDelegateForFunctionPointer<T>(address);
    }

    private unsafe delegate IntPtr CreateImage(IntPtr display, IntPtr context, int target, IntPtr buffer,
        int* attributes);

    private delegate int DestroyImage(IntPtr display, IntPtr image);

    [DllImport("libEGL.so.1")]
    private static extern IntPtr eglGetCurrentDisplay();

    /// <summary>
    /// What a driver answers when it has no name for the layout it exported: DRM_FORMAT_MOD_INVALID.
    /// The frames carry whatever tiling the driver picked, and an import that states nothing resolves it the
    /// same way the export did.
    /// </summary>
    private const ulong ImplicitModifier = 0x00ffffffffffffff;

    private const int EGL_HEIGHT = 0x3056;
    private const int EGL_WIDTH = 0x3057;
    private const int EGL_NONE = 0x3038;
    private const int EGL_LINUX_DMA_BUF_EXT = 0x3270;
    private const int EGL_LINUX_DRM_FOURCC_EXT = 0x3271;
    private const int EGL_DMA_BUF_PLANE0_FD_EXT = 0x3272;
    private const int EGL_DMA_BUF_PLANE0_OFFSET_EXT = 0x3273;
    private const int EGL_DMA_BUF_PLANE0_PITCH_EXT = 0x3274;
    private const int EGL_DMA_BUF_PLANE0_MODIFIER_LO_EXT = 0x3443;
    private const int EGL_DMA_BUF_PLANE0_MODIFIER_HI_EXT = 0x3444;

    /// <summary>The one drawing mode this surface uses, which <c>GlConsts</c> does not carry.</summary>
    private const int GL_TRIANGLE_STRIP = 0x0005;
}
