using ScreenShare.Api.V1;

namespace ScreenShare.App.Backend;

/// <summary>
/// What the preset store answered: the configurations the user saved, and the notice standing in for the ones
/// that could not be read.
/// The two travel together: an empty list means either nothing saved or nothing readable, and only the notice
/// tells the two apart.
/// A reading that dropped it leaves the screen saying "no presets" to someone whose presets are still on disk.
/// </summary>
/// <param name="Saved">In the order the store holds them.</param>
/// <param name="Notice">Why the store could not be read, carrying where the unreadable file was kept
/// (<c>ListPresetsResponse</c>). null when read.</param>
public sealed record PresetStore(IReadOnlyList<Preset> Saved, Text? Notice)
{
    /// <summary>Store nothing has read, what a screen opens on.</summary>
    public static readonly PresetStore Unread = new([], null);
}
