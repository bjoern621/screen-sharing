using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// Last measurement each figure of the running stream stated, so a pass that measures none prints what the last
/// one did instead of <see cref="Figure.NoValue"/>.
///
/// A sample states a figure only where something measured it over that interval, and a second is short enough
/// for that to fail on a healthy stream: an encoder that emitted no frame in one times nothing, and a relay poll
/// that has not landed states no reader.
/// Both are gaps in a figure that is otherwise there, and a row alternating between a number and an ellipsis
/// is a row nobody reads.
///
/// Held for the run and no further.
/// A stream that stopped, one waiting out a retry and one publishing under another name measure nothing at all,
/// so every held figure goes and the screen reads absent.
///
/// The two relay figures are held only while the relay states no path.
/// A path naming readers and timing none of them is the answer rather than a gap, which the header says
/// in a sentence (<c>HeaderStats/ViewModel/HeaderStatsViewModel.cs</c>), and a number held over that sentence would
/// name a round trip nobody is taking.
/// </summary>
public sealed class HeldFigures
{
    private string _stream = "";
    private double? _egress;
    private double? _fps;
    private double? _encode;
    private double? _loss;
    private int? _rtt;
    private int? _viewers;

    /// <summary>
    /// The reading with each unmeasured figure filled from the last pass that measured it.
    /// Safe to run twice on one reading: a filled figure is recorded as the value it was filled with.
    /// </summary>
    public BroadcastSnapshot Fill(BroadcastSnapshot reading)
    {
        Assert.NotNull(reading, "a hold fills the reading it is handed");

        // A stream that stopped and one waiting out a retry measure nothing, so there is nothing to fill from.
        if (!reading.IsLive || reading.IsRetrying)
        {
            Forget(reading.Stream);
            return reading;
        }

        // A stream under another name is another run, and its first pass still measures: forgetting what
        // the last one read is the whole of it.
        if (reading.Stream != _stream)
        {
            Forget(reading.Stream);
        }

        // Read before anything is filled: what it answers is whether the relay stated a path this pass, and
        // a held count would make an unreachable relay look like one naming readers.
        var stated = reading.Viewers is not null;

        var filled = reading with
        {
            EgressMbps = Held(ref _egress, reading.EgressMbps),
            Fps = Held(ref _fps, reading.Fps),
            EncodeMs = Held(ref _encode, reading.EncodeMs),
            Viewers = Held(ref _viewers, reading.Viewers),
            RttMs = stated ? Stated(ref _rtt, reading.RttMs) : Held(ref _rtt, reading.RttMs),
            LossPercent = stated ? Stated(ref _loss, reading.LossPercent) : Held(ref _loss, reading.LossPercent),
        };

        Assert.That(filled.IsLive && filled.Stream == reading.Stream,
            "a held figure describes the run that measured it", reading.Stream);
        return filled;
    }

    /// <summary>Drops every held figure and takes the run they will be measured on.</summary>
    private void Forget(string stream)
    {
        _stream = stream;
        _egress = _fps = _encode = _loss = null;
        _rtt = null;
        _viewers = null;
    }

    /// <summary>What this pass measured, or the last pass that measured anything.</summary>
    private static T? Held<T>(ref T? held, T? measured) where T : struct
    {
        if (measured is not null)
        {
            held = measured;
        }

        return measured ?? held;
    }

    /// <summary>What this pass measured, absence included, for a figure whose source states both.</summary>
    private static T? Stated<T>(ref T? held, T? measured) where T : struct
    {
        held = measured;
        return measured;
    }
}
