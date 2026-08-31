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
/// Import path for a dmabuf descriptor: EGL opens it here and a quad draws the texture.
///
/// Avalonia's compositor takes a shared texture or an opaque descriptor and no dmabuf,
/// so nothing hands this handle type over.
/// The import is EGL's, <c>eglCreateImageKHR(EGL_LINUX_DMA_BUF_EXT)</c> with <c>glEGLImageTargetTexture2DOES</c>,
/// and the compositor receives this control's drawing (<c>docs/viewer-architecture.md</c>, "The frame channel").
///
/// <see cref="OpenGlControlBase"/> paints into a composition surface instead of a window of its own,
/// so a menu or a figure above a tile stays above it, which a native child window would break
/// (<c>avalonia/README.md</c>).
///
/// No pixel read: the descriptor names memory the decoder wrote, the driver samples it in place,
/// and the per-frame cost here is one quad.
///
/// EGL only, a requirement of the handle type.
/// GLX exposes no import for a descriptor, so a tile on such a window carries a sentence and draws nothing.
/// Both halves of the app steer their GL to EGL:
/// <c>GST_GL_PLATFORM</c> in the backend, EGL ahead of GLX in <c>X11RenderingMode</c> (<c>Program.cs</c>).
/// </summary>
internal sealed class DmaBufSurface : OpenGlControlBase, ITileSurface
{
    /// <summary>
    /// A pool as this surface imports it: the layout, and one descriptor per slot.
    /// Record, so identity alone separates an imported pool from a lent one, holding the import to once per pool.
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

    /// <summary>Pool for the next pass, and the one already imported. Apart for exactly one pass.</summary>
    private Lent? _lent;
    private Lent? _imported;

    /// <summary>
    /// Imports by slot index.
    /// A descriptor becomes a texture once and is drawn from repeatedly, what the pool is for,
    /// an import per frame being a driver crossing per frame.
    /// </summary>
    private readonly List<int> _textures = [];
    private readonly List<IntPtr> _images = [];

    /// <summary>Which slot the next pass draws. Unset until a frame arrives.</summary>
    private uint? _slot;

    /// <summary>
    /// Ask outstanding on the renderer.
    /// A single pass answers every ask that preceded it, a caller needing only a draw later than its ask.
    /// </summary>
    private TaskCompletionSource<string?>? _pass;

    /// <summary>Sentence a tile shows in place of a picture. Null while frames draw.</summary>
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
        // Descriptors ahead of the pass, never inside it: reading them is a socket round trip with another process,
        // and a render pass cannot block on one.
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
    /// Requests a pass and waits on it.
    ///
    /// Waiting is what the loan is made of: a slot is free once the draw reading it completed on the device,
    /// where this resumes.
    ///
    /// The bound covers the one failure that would otherwise be silent:
    /// a renderer that never comes up calls no pass,
    /// leaving a tile that holds a slot and reports nothing for the rest of the run.
    /// Past the bound the slot returns and the tile carries a reason.
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
    /// Ceiling on one pass.
    /// Above the time a renderer spends building its first context and shaders,
    /// below the patience of somebody looking at an empty tile.
    /// </summary>
    private static readonly TimeSpan PassBound = TimeSpan.FromSeconds(5);

    protected override void OnOpenGlInit(GlInterface gl)
    {
        // Only a current context has a display, so it is captured here.
        // Zero where libEGL is absent, which the import turns into a notice.
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

        // Whoever is waiting is told, instead of being left with a slot that can be neither drawn nor released.
        _failure = "This tile's renderer went away.";
        Answer();
    }

    /// <summary>
    /// Context lost, and with it every import made on it.
    /// No delete is issued: a texture and an image are freed on the context that owned them.
    /// What goes is the names, so no pass samples one that means nothing,
    /// and the pool with them, a second import wanting descriptors the first one closed.
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
    /// Releases the held pool, closing descriptors no import consumed.
    /// Keeping the pool would skip the import on a repeat of the same object,
    /// whose descriptors are closed as soon as an import has read them.
    /// </summary>
    private void Forget()
    {
        ReleaseUnimported();
        _lent = null;
        _imported = null;
    }

    /// <summary>
    /// Closes descriptors of a pool no pass ever imported, which the import itself would have closed.
    /// Reached where two pools land inside one render pass, a stream renegotiating twice while the window was busy.
    /// </summary>
    private void ReleaseUnimported()
    {
        if (_lent is not null && !ReferenceEquals(_lent, _imported))
        {
            FrameDescriptors.Release(_lent.Descriptors);
        }
    }

    /// <summary>
    /// The one render function: import what was lent, draw the slot named, answer the ask.
    /// A pass writes the whole surface, no path preserving the previous frame,
    /// so a pool with nothing drawable clears to black instead of leaving up a picture from a pool that is gone.
    /// </summary>
    protected override void OnOpenGlRender(GlInterface gl, int fb)
    {
        Import(gl);
        Paint(gl);

        // Finish, not flush: this pass answering returns the slot,
        // and a queued draw would sample memory the next frame is landing in.
        gl.Finish();
        Answer();
    }

    /// <summary>Imports the lent pool. Does nothing where it is already the imported one.</summary>
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
                _failure = "This window's renderer cannot open the kind of shared frame this computer decodes into.";
                return;
            }

            for (var slot = 0; slot < lent.Descriptors.Length; slot++)
            {
                if (!Import(gl, lent, slot))
                {
                    Drop(gl);
                    _failure = "The frames this computer decoded could not be opened by this window's renderer.";
                    return;
                }
            }
            _failure = null;
        }
        finally
        {
            // These descriptors belong to this process, and EGL references the memory independently of them,
            // so they close as soon as the import has read them, whether or not it succeeded.
            // The image holds the memory afterwards.
            FrameDescriptors.Release(lent.Descriptors);
        }
    }

    /// <summary>One descriptor into one texture, under the layout the pool stated.</summary>
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
            // Passed only where the exporter named a layout.
            // The implicit value means the driver chose,
            // so naming a modifier here would assert a tiling nobody picked.
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

        // Linear and clamped: a tile draws at whatever size it was arranged at,
        // and rounding the render size onto a rung usually leaves the picture larger than its box.
        gl.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
        gl.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
        gl.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
        gl.TexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
        gl.BindTexture(GL_TEXTURE_2D, 0);

        _textures.Add(texture);
        _images.Add(image);
        return true;
    }

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
        // Taken from the pool, never assumed: row zero of a texture is where OpenGL puts the bottom of the picture,
        // and a backend whose frames start at the top is what this uniform answers.
        gl.Uniform1f(_flipUniform, _imported.TopLeftOrigin ? 1 : 0);

        gl.BindVertexArray(_vertexArray);
        gl.BindBuffer(GL_ARRAY_BUFFER, _vertices);
        gl.EnableVertexAttribArray(_cornerAttribute);
        gl.VertexAttribPointer(_cornerAttribute, 2, GL_FLOAT, 0, 2 * sizeof(float), IntPtr.Zero);
        gl.DrawArrays(GL_TRIANGLE_STRIP, 0, new IntPtr(4));

        gl.BindBuffer(GL_ARRAY_BUFFER, 0);
        gl.BindVertexArray(0);
    }

    private void Answer()
    {
        var pass = _pass;
        _pass = null;
        pass?.TrySetResult(_failure);
    }

    /// <summary>
    /// Deletes the imports of a replaced pool.
    /// Wants the owning context current, so it runs inside a pass or in init and deinit.
    /// </summary>
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
    /// Compiles the program and uploads the quad.
    /// Shader instead of a blit: <c>glBlitFramebuffer</c> wants OpenGL ES 3
    /// while this control takes whatever context the window's renderer built,
    /// and no blit flips a picture that starts at its top row.
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
    /// Source in the dialect this context speaks.
    /// Written once in the older dialect, the newer one renaming a few keywords and the texture lookup.
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
        // GL objects wait for the control's own deinit, the only place with a context to free them on.
        // The ask ends here: a tile that let this surface go asks nothing more,
        // and an unanswered pass would outlive it.
        Forget();
        _slot = null;
        Answer();
        return ValueTask.CompletedTask;
    }

    /// <summary>Pool format as the DRM fourcc an import names it by.</summary>
    private static int FourccOf(FrameFormat format) => format switch
    {
        // A fourcc spells the channels in reverse of their order in memory:
        // DRM_FORMAT_ABGR8888 puts red in the first byte.
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
    /// DRM_FORMAT_MOD_INVALID, the answer of a driver that named no layout for what it exported.
    /// Whatever tiling it picked travels with the frames,
    /// and an import naming no modifier resolves it the way the export did.
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

    /// <summary>Drawing mode missing from <c>GlConsts</c>.</summary>
    private const int GL_TRIANGLE_STRIP = 0x0005;
}
