using ScreenShare.App.Features.Setup.Model;

namespace ScreenShare.App.Features.Tray.Model;

/// <summary>Which half of the preset card a tray row came from, deciding how picking it applies.</summary>
public enum TrayPresetKind
{
    /// <summary>Promise about the picture, applied off the form the window holds (<c>docs/presets.md</c>).</summary>
    Builtin,

    /// <summary>Named snapshot, applied off the store's last answer.</summary>
    Saved,
}

/// <summary>
/// One preset as the tray menu draws it.
/// A record of scalars, so two passes over one card compare equal and an unchanged menu notifies nothing.
/// </summary>
public sealed record TrayPresetEntry
{
    public required TrayPresetKind Kind { get; init; }

    /// <summary>Built-in key or saved name, what picking the row is dispatched on.</summary>
    public required string Id { get; init; }

    public required string Name { get; init; }

    /// <summary>Whether the draft is this preset, as the card derived it on this pass.</summary>
    public required bool IsCurrent { get; init; }

    /// <summary>False on a built-in promise nothing on this machine reaches, drawn greyed.</summary>
    public required bool IsReachable { get; init; }
}

/// <summary>
/// The tray menu's whole state: the one commit row, and a row per preset, built-ins before saved ones.
///
/// Equality is by content, the entry list included, so the view model's render pass can assign every pass
/// and an unchanged menu raises nothing (<c>docs/development-principles.md</c>, "Idempotency").
/// </summary>
public sealed record TrayMenu
{
    /// <summary>Whether a stream is on the air, which also picks the tray icon.</summary>
    public required bool IsLive { get; init; }

    /// <summary>Start sharing or stop sharing, following what committing would do.</summary>
    public required string CommitLabel { get; init; }

    public required bool CanCommit { get; init; }

    public required IReadOnlyList<TrayPresetEntry> Presets { get; init; }

    /// <summary>Menu before anything has been read: a start nothing has vouched for yet, and no presets.</summary>
    public static readonly TrayMenu Unread = new()
    {
        IsLive = false,
        CommitLabel = CommitCopy.Of(PublishCommit.Start).Label,
        CanCommit = false,
        Presets = [],
    };

    public bool Equals(TrayMenu? other)
        => other is not null
           && IsLive == other.IsLive
           && CommitLabel == other.CommitLabel
           && CanCommit == other.CanCommit
           && Presets.SequenceEqual(other.Presets);

    public override int GetHashCode()
        => HashCode.Combine(IsLive, CommitLabel, CanCommit, Presets.Count);
}
