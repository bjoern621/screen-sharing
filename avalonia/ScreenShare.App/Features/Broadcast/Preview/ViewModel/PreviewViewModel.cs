using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.Preview.ViewModel;

/// <summary>
/// The program preview: what viewers see, and what it costs to send it.
///
/// There is no video pipeline behind it yet, so the tile draws a placeholder rather than
/// a frame. The overlay figures are real derivations from the reading, which is what makes
/// the preview answer "is the encoder keeping up" and not only "is something on air".
/// </summary>
public sealed class PreviewViewModel : Observable
{
    public PreviewViewModel() => Apply();

    // --- Inputs -------------------------------------------------------------------

    private BroadcastSnapshot _snapshot = BroadcastSnapshot.Unread;

    public BroadcastSnapshot Snapshot
    {
        get => _snapshot;
        set
        {
            Assert.NotNull(value, "a preview renders a reading");

            if (Set(ref _snapshot, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private string _placeholder = "";
    private string _encoded = "";
    private string _quality = "";
    private bool _isOnAir;

    /// <summary>What stands in for the frame: what the tile is, and at what size.</summary>
    public string Placeholder { get => _placeholder; private set => Set(ref _placeholder, value); }

    public string Encoded { get => _encoded; private set => Set(ref _encoded, value); }

    public string Quality { get => _quality; private set => Set(ref _quality, value); }

    /// <summary>
    /// Whether the inset red outline and the Program badge show. Both mean one thing -
    /// this tile is what the world is currently receiving - so both follow the same fact.
    /// </summary>
    public bool IsOnAir { get => _isOnAir; private set => Set(ref _isOnAir, value); }

    /// <summary>The one render function. Sets the off branch too, so the outline cannot stick.</summary>
    public void Apply()
    {
        var reading = Snapshot;

        IsOnAir = reading.IsLive;

        // The size is joined only where the stream states one: an empty output resolution means
        // the source's own size, which this shell has no way to name, so the placeholder says
        // what the tile is and stops rather than trailing a separator with nothing after it.
        Placeholder = reading.Resolution.Length > 0
            ? Figure.Join("What viewers see", reading.Resolution)
            : "What viewers see";
        Encoded = $"encoded {Figure.Of(reading.Fps, "0.0")} fps";
        Quality = $"cq {Figure.Of(reading.Cq)}";

        Assert.That(Placeholder.Length > 0, "a preview always names what it stands in for");
    }
}
