using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The audio source list, as the shell writes it.
///
/// A form field key is an address into the settings message, and a list entry makes it three steps rather
/// than two: the group, the entry of a repeated field, the field inside it.
/// What these lock out is the shell learning anything else about the list.
/// It appends no entry it was not addressed to, it decides nothing about which entry is which, and it looks
/// copy up by the control rather than by the entry (docs/ipc-api.md, "The rule").
/// </summary>
public sealed class AudioListTests
{
    private static Settings Draft() => new() { Publish = new PublishSettings() };

    [Fact]
    public void AWriteReachesTheEntryTheKeyNames()
    {
        var draft = Draft();
        draft.Publish.AudioSources.Add(new AudioSource { Source = "desktop", Gain = 100 });
        draft.Publish.AudioSources.Add(new AudioSource { Source = "mic", Gain = 100 });

        SettingsDraft.Write(draft, "publish.audio_sources[1].gain", new FieldValue { Number = 150 });

        Assert.Equal(100, draft.Publish.AudioSources[0].Gain);
        Assert.Equal(150, draft.Publish.AudioSources[1].Gain);
    }

    /// <summary>
    /// The row the form draws past the end of the list is what a reader grows it by, so a write through its
    /// key adds the entry.
    /// That is the whole of adding a source: an ordinary settings write through an ordinary control, with no
    /// effect on the contract for it.
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
    /// A fresh entry carries no level, which is unity rather than silence: the field carries presence exactly
    /// so that a source a reader has just added is not one nobody can hear.
    /// </summary>
    [Fact]
    public void AFreshEntryCarriesNoLevelOfItsOwn()
    {
        var draft = Draft();

        SettingsDraft.Write(draft, "publish.audio_sources[0].source", new FieldValue { Text = "desktop" });

        Assert.False(draft.Publish.AudioSources[0].HasGain);
    }

    /// <summary>
    /// A write past the end of the list is refused rather than filling the gap, because the entries between
    /// would be entries nobody chose.
    /// </summary>
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
        SettingsDraft.Write(draft, "publish.audio_sources[0].source", new FieldValue { Text = "mic" });
        SettingsDraft.Write(draft, "publish.audio_sources[0].mute", new FieldValue { Flag = true });

        Assert.Equal("mic", SettingsDraft.Read(draft, "publish.audio_sources[0].source").Text);
        Assert.True(SettingsDraft.Read(draft, "publish.audio_sources[0].mute").Flag);
    }

    /// <summary>
    /// Copy is written for the control and never for one entry, because the third microphone's level means
    /// what the first one's does.
    /// </summary>
    [Fact]
    public void EveryEntryOfAListSharesTheControlsCopy()
    {
        var first = Fields.Of("publish.audio_sources[0].gain");
        var third = Fields.Of("publish.audio_sources[2].gain");

        Assert.Equal(first, third);
        Assert.NotEqual("publish.audio_sources[0].gain", first.Label);
    }

    /// <summary>
    /// The entries of a list control are named the same way whichever entry they are on, which is the same
    /// normalisation the copy lookup makes.
    /// </summary>
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
