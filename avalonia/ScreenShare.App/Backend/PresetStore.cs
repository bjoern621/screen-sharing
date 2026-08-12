using ScreenShare.Api.V1;

namespace ScreenShare.App.Backend;

/// <summary>
/// What the preset store answered: the configurations the user saved, and the notice
/// standing in for the ones that could not be read.
///
/// <b>The two travel together because an empty list means two different things.</b> Nothing
/// has been saved yet, and nothing readable remained, are different facts about the reader's
/// machine, and only the notice tells them apart - it also carries where the unreadable file
/// was kept (<c>ListPresetsResponse</c>). A reading that dropped it would leave the screen
/// saying "no presets" to someone whose presets are still on disk.
/// </summary>
/// <param name="Saved">The presets, in the order the store holds them.</param>
/// <param name="Notice">Why the store could not be read, null when it was.</param>
public sealed record PresetStore(IReadOnlyList<Preset> Saved, Text? Notice)
{
    /// <summary>A store nothing has read yet, which is what a screen opens on.</summary>
    public static readonly PresetStore Unread = new([], null);
}
