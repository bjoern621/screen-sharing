using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Fields.Model;

/// <summary>
/// Which screen draws which group of the resolved form.
///
/// <b>Placement, which is the shell's whole business.</b> Which groups exist, in what order,
/// with which controls under each, is the backend's answer and is not restated here
/// (<c>docs/ipc-api.md</c>, "The rule"). What the contract cannot see is that this shell has
/// three destinations and that one of them is about sending and another about receiving - and a
/// group of receiving settings drawn inside the sending wizard is a step the reader has to walk
/// past to reach the one they came for.
///
/// The split is by what the reader is doing, and the form's own grouping already draws it: the
/// watch group holds the legs a stream comes back on, the jitter buffers a receiver holds and
/// the chain a tile converts frames with. None of them changes a single byte this machine sends,
/// and every one of them changes what a tile in the viewer does. So the viewer draws it, where
/// the tiles it governs are, and the wizard draws everything else.
///
/// <b>The default is the wizard, and that is deliberate.</b> A group the backend adds is a step
/// that appears and works with nothing here to edit, which is the property
/// <see cref="Setup.Model.SetupSteps"/> exists to keep. Only the one group named below is
/// answered differently, so this table can never make a group unreachable - the two predicates
/// are a partition by construction.
/// </summary>
public static class GroupPlacement
{
    /// <summary>
    /// The group about receiving rather than sending: the two watch legs, the two jitter buffers
    /// and the render chain. It is the form's own key for it.
    /// </summary>
    public const string WatchKey = "watch";

    /// <summary>Whether the setup wizard draws this group as one of its steps.</summary>
    public static bool InSetup(string key)
    {
        Assert.That(key.Length > 0, "placing a group names the group being placed");

        return !InViewer(key);
    }

    /// <summary>
    /// Whether the viewer draws this group beside the tiles it governs. The complement of
    /// <see cref="InSetup"/>, so between them every group the form carries is drawn once.
    /// </summary>
    public static bool InViewer(string key)
    {
        Assert.That(key.Length > 0, "placing a group names the group being placed");

        return key == WatchKey;
    }
}
