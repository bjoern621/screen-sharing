using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Audio source list, as the shell writes into it.
///
/// A field key addresses the settings message, and a list entry makes that address three steps rather than
/// two: the group, the entry of a repeated field, the field inside it.
///
/// What these lock out is the shell knowing anything more about the list.
/// It appends no entry it was not addressed to, decides nothing about which entry is which, and looks copy up
/// by the control rather than by the entry (docs/ipc-api.md, "The rule").
/// </summary>
public sealed class AudioListTests
{
    private static Settings Draft() => new() { Publish = new PublishSettings() };

    [Fact]
    public void AWriteReachesTheEntryTheKeyNames()
    {
        var draft = Draft();
        draft.Publish.AudioSources.Add(new AudioSource { Source = "desktop", Gain = 100 });
        draft.Publish.AudioSources.Add(new AudioSource { Source = "application", Gain = 100 });

        SettingsDraft.Write(draft, "publish.audio_sources[1].gain", new FieldValue { Number = 150 });

        Assert.Equal(100, draft.Publish.AudioSources[0].Gain);
        Assert.Equal(150, draft.Publish.AudioSources[1].Gain);
    }

    /// <summary>
    /// The row the form draws past the end is what a reader grows the list by.
    /// Adding a source is therefore an ordinary settings write through an ordinary control, with no effect of
    /// its own on the contract.
    /// </summary>
    [Fact]
    public void AWriteOnePastTheEndAddsTheEntry()
    {
        var draft = Draft();

        SettingsDraft.Write(draft, "publish.audio_sources[0].source", new FieldValue { Text = "desktop" });

        var entry = Assert.Single(draft.Publish.AudioSources);
        Assert.Equal("desktop", entry.Source);
    }

    /// <summary>
    /// An absent level is unity rather than silence.
    /// The field carries presence for that reason: a source a reader has just added is not one nobody can
    /// hear.
    /// </summary>
    [Fact]
    public void AFreshEntryCarriesNoLevelOfItsOwn()
    {
        var draft = Draft();

        SettingsDraft.Write(draft, "publish.audio_sources[0].source", new FieldValue { Text = "desktop" });

        Assert.False(draft.Publish.AudioSources[0].HasGain);
    }

    /// <summary>Filling the gap instead would create entries nobody chose.</summary>
    [Fact]
    public void AWriteBeyondTheGrowingRowIsRefused()
    {
        var draft = Draft();

        Assert.ThrowsAny<Exception>(() =>
            SettingsDraft.Write(draft, "publish.audio_sources[3].source", new FieldValue { Text = "desktop" }));
    }

    [Fact]
    public void AnEntryReadsBackWhatWasWrittenToIt()
    {
        var draft = Draft();
        SettingsDraft.Write(draft, "publish.audio_sources[0].source", new FieldValue { Text = "application" });
        SettingsDraft.Write(draft, "publish.audio_sources[0].mute", new FieldValue { Flag = true });

        Assert.Equal("application", SettingsDraft.Read(draft, "publish.audio_sources[0].source").Text);
        Assert.True(SettingsDraft.Read(draft, "publish.audio_sources[0].mute").Flag);
    }

    /// <summary>Copy is written per control, since the third source's level means what the first one's does.</summary>
    [Fact]
    public void EveryEntryOfAListSharesTheControlsCopy()
    {
        var first = Fields.Of("publish.audio_sources[0].gain");
        var third = Fields.Of("publish.audio_sources[2].gain");

        Assert.Equal(first, third);
        Assert.NotEqual("publish.audio_sources[0].gain", first.Label);
    }

    /// <summary>Option names normalise the entry index away, as the copy lookup does.</summary>
    [Fact]
    public void AnEntrysOptionsAreNamedByTheControl()
    {
        var words = Vocabulary.Empty;

        Assert.Equal(
            words.Name("publish.audio_sources[0].source", "desktop"),
            words.Name("publish.audio_sources[4].source", "desktop"));
        Assert.NotEqual("desktop", words.Name("publish.audio_sources[0].source", "desktop"));
    }
}
