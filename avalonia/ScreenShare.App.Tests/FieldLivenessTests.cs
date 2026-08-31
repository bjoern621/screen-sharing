using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What a control costs to change while people are watching.
/// Liveness moves with the engine, the codec and the rate-control mode,
/// so the backend answers it per control and the shell repeats that answer (<c>docs/ipc-api.md</c>, "The rule").
/// A list of live keys held here promises a reconnect-free edit the backend does not deliver.
/// </summary>
public sealed class FieldLivenessTests
{
    private static FieldViewModel Rendered(bool live)
    {
        var field = new FieldViewModel("publish.bitrate_mbps", (_, _) => { });
        field.Apply(
            new Field
            {
                Key = "publish.bitrate_mbps",
                Control = ControlKind.Number,
                Visible = true,
                Enabled = true,
                Live = live,
                Value = new FieldValue { Number = 20 },
            },
            Vocabulary.Empty);
        return field;
    }

    [Fact]
    public void AControlTheBackendMarkedLiveSaysWhatChangingItCosts()
    {
        var field = Rendered(true);

        Assert.True(field.AppliesLive);
        Assert.Equal(Fields.LiveNotice, field.LiveNotice);
    }

    [Fact]
    public void AControlTheBackendDidNotMarkPromisesNothing()
    {
        Assert.False(Rendered(false).AppliesLive);
    }

    /// <summary>A mode that sends the encoder no rate takes liveness off a control carrying it.</summary>
    [Fact]
    public void ASecondPassTakesTheFlagBack()
    {
        var field = Rendered(true);
        field.Apply(
            new Field
            {
                Key = "publish.bitrate_mbps",
                Control = ControlKind.Number,
                Visible = true,
                Enabled = true,
                Live = false,
                Value = new FieldValue { Number = 20 },
            },
            Vocabulary.Empty);

        Assert.False(field.AppliesLive);
    }
}
