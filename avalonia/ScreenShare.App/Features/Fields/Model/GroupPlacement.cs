using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Fields.Model;

/// <summary>
/// Which destination draws which group of the resolved form.
///
/// Placement is the shell's, and the contract describes no screens.
/// Which groups exist, in what order and with what under each is the backend's (<c>docs/ipc-api.md</c>, "The rule").
///
/// The watch group holds the legs a stream comes back on, the jitter buffers a receiver holds
/// and the chain a tile converts frames with.
/// None of it changes a byte this machine sends, so the viewer draws it beside the tiles it governs
/// and the wizard draws every other group.
///
/// Defaulting to the wizard makes a group the backend adds a step that appears with nothing here to edit
/// (<see cref="Setup.Model.SetupSteps"/>).
/// </summary>
public static class GroupPlacement
{
    /// <summary>The receiving group as the form names it: the watch legs, the jitter buffers,
    /// the render chain.</summary>
    public const string WatchGroup = "watch";

    public static bool InSetup(string key)
    {
        Assert.That(key.Length > 0, "placing a group names the group being placed");

        return !InViewer(key);
    }

    /// <summary>Complement of <see cref="InSetup"/>, so every group the form carries is drawn exactly once.</summary>
    public static bool InViewer(string key)
    {
        Assert.That(key.Length > 0, "placing a group names the group being placed");

        return key == WatchGroup;
    }
}
