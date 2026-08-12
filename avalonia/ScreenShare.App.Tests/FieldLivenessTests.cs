using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What a control costs to change while people are watching.
///
/// The backend answers it per control and per combination, and this side repeats the answer.
/// What these lock out is the shell growing a list of its own: the flag moves with the
/// engine, the codec and the rate-control mode, so a list held here would go on promising a
/// reconnect-free edit after the backend stopped delivering one (docs/ipc-api.md, "The rule").
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

    /// <summary>
    /// The flag is an output like every other, written on every pass. A control that stopped
    /// being live - because the mode moved to one that sends the encoder no rate - has to stop
    /// saying so through the same render function that started saying it.
    /// </summary>
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
