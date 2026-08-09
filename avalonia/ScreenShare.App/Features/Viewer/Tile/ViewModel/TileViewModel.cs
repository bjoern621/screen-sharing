using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.Tile.ViewModel;

/// <summary>
/// One tile of the grid: the stream it draws, the figures over it, and the subscription the
/// control behind it reads.
///
/// <b>It holds two kinds of figure and they come from opposite directions.</b> What the
/// pipeline is - the render chain that ran, the memory at each end, the decoder - is the
/// backend's and is read through <see cref="Session.Receiving"/> on every pass, like every
/// other state this shell draws. What this window got and drew is the tile's own and can come
/// from nowhere else: a backend cannot see that a compositor was too slow to take a frame.
/// The second kind arrives through <see cref="Report"/> and is the only state this class
/// stores.
///
/// <b>It decides nothing about the decode.</b> Whether a stream is being received is
/// <c>StartReceive</c>'s answer and the roster's business, and whether the publish's local
/// preview is running is the publish's; this draws whatever the subscription hands it and
/// says why when it is handed nothing.
///
/// <b>It is one class for both kinds of tile, and that is deliberate.</b> The viewer's grid
/// draws relay decodes and the broadcast screen draws the local preview, and the difference
/// between them is entirely in what a subscription names (<see cref="TileSource"/>) and where
/// the pipeline's own state is read from (<see cref="TilePipeline"/>). A second implementation
/// would be a second answer to what a dropped frame is and where a lent handle goes back.
/// </summary>
public sealed class TileViewModel : Observable, IFrameSource
{
    private readonly TileSource _source;
    private readonly IBackend _backend;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// What the control last reported. The one piece of state here, and it is this window's own.
    ///
    /// It starts at <see cref="TileReport.Nothing"/> rather than at the struct's default,
    /// because a tile is rendered as soon as it is added and its first report arrives after
    /// that: a default carries a null notice, and the render reads the notice.
    /// </summary>
    private TileReport _report = TileReport.Nothing;

    public TileViewModel(TileSource source, IBackend backend, Action<Action> dispatch)
    {
        Assert.NotNull(source, "a tile names the decode it draws from");
        Assert.NotNull(backend, "a tile subscribes to the backend's frames");
        Assert.NotNull(dispatch, "a tile needs a UI loop to marshal a report back to");

        _source = source;
        _backend = backend;
        _dispatch = dispatch;
    }

    /// <summary>Which decode this tile draws from, read through rather than taken apart.</summary>
    public TileSource Source => _source;

    /// <summary>The stream name, carried and never parsed.</summary>
    public string Name => _source.Name;

    /// <summary>The leg the decode this tile draws was opened on, empty for the local preview.</summary>
    public string Transport => _source.Transport;

    // --- Outputs ---------------------------------------------------------------------

    private string _title = "";
    private string _notice = "";
    private bool _hasNotice;
    private bool _isLive;

    /// <summary>The stream and the leg, as the tile's heading prints them.</summary>
    public string Title { get => _title; private set => Set(ref _title, value); }

    /// <summary>
    /// What the pipeline and this window turned out to be, as the strip under the picture
    /// prints it. A list rather than named slots, so what a tile reports stays the tile's.
    /// </summary>
    public IReadOnlyList<string> Figures { get; private set; } = [];

    /// <summary>Why the tile is dark, empty while it is drawing.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>Whether a frame has been drawn, which separates a live tile from a connecting one.</summary>
    public bool IsLive { get => _isLive; private set => Set(ref _isLive, value); }

    // --- Lifecycle -------------------------------------------------------------------

    /// <summary>
    /// The one render function. Reads the decode's state through on every pass and combines it
    /// with the report the control last made, so an unchanged pair fires no binding.
    /// </summary>
    public void Apply(TilePipeline? pipeline)
    {
        // The preview's frames crossed no protocol, so there is none to name beside the
        // stream. Everything else on the heading is the same fact either way.
        Title = _source.IsPreview ? Name : $"{Name} · {Words.Transport(Transport)}";
        IsLive = _report.Live;

        Notice = NoticeFor(pipeline);
        HasNotice = Notice.Length > 0;
        Figures = FiguresFor(pipeline);

        Assert.That(HasNotice == (Notice.Length > 0), "a notice and its sentence agree", Name);
    }

    /// <inheritdoc />
    public Task<FrameChannel> OpenAsync(CancellationToken cancellation)
        => _source.OpenAsync(_backend, cancellation);

    /// <inheritdoc />
    public void Report(TileReport report)
    {
        // The control is on the UI loop already, but the dispatch is kept rather than assumed:
        // it is the one guarantee that a report and an event-driven pass cannot interleave.
        _dispatch(() =>
        {
            _report = report;
            Changed?.Invoke();
        });
    }

    /// <summary>Raised after a report has moved, so the screen holding this tile re-renders.</summary>
    public event Action? Changed;

    /// <summary>
    /// Why this tile is dark.
    ///
    /// Three states a reader has to be able to tell apart, and the order is the order they
    /// happen in: nothing is decoding this pair, the pipeline is up and no frame has left it,
    /// or the tile itself could not draw what it was handed. The last one is the control's own
    /// sentence and is shown as it stands, because it names a driver or a handle type and
    /// nothing else here knows either.
    /// </summary>
    private string NoticeFor(TilePipeline? pipeline)
    {
        if (_report.Notice.Length > 0)
        {
            return _report.Notice;
        }

        if (pipeline is null)
        {
            return "Nothing is decoding this stream.";
        }

        return pipeline.Value.Live ? "" : "Connecting.";
    }

    /// <summary>
    /// What the strip prints: what the pipeline turned out to be, then what this window did
    /// with it.
    ///
    /// The memory pair is the figure worth having and the reason the backend reports both
    /// ends. A chain that promised to keep the frames on the device and a decoder that
    /// downloaded its own output disagree, and the pair is the only place that shows.
    /// </summary>
    private IReadOnlyList<string> FiguresFor(TilePipeline? state)
    {
        if (state is not { } decode)
        {
            return [];
        }

        var figures = new List<string>(5);

        if (_report.Width > 0)
        {
            figures.Add($"{_report.Width}×{_report.Height}");
        }

        if (decode.Decoder.Length > 0)
        {
            figures.Add(decode.Hardware ? $"{decode.Decoder} on the GPU" : decode.Decoder);
        }

        if (decode.Chain.Length > 0)
        {
            figures.Add($"{decode.Chain} chain");
        }

        if (decode.RenderMemory.Length > 0)
        {
            figures.Add(MemoryLabel(decode.RenderMemory));
        }

        if (_report.Dropped > 0)
        {
            figures.Add($"{_report.Dropped} dropped");
        }

        return figures;
    }

    /// <summary>
    /// How a memory feature reads on screen. The identifiers are GStreamer's own and cross the
    /// contract unchanged; what they are called here is this shell's, like every other word.
    /// </summary>
    private static string MemoryLabel(string memory) => memory switch
    {
        "memory:SystemMemory" => "frames in system memory",
        "memory:D3D11Memory" => "frames on the GPU, Direct3D 11",
        "memory:D3D12Memory" => "frames on the GPU, Direct3D 12",
        "memory:GLMemory" => "frames on the GPU, OpenGL",
        "memory:DMABuf" => "frames on the GPU, dmabuf",
        _ => memory,
    };
}
