using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What a control says when this combination leaves it nothing to pick.
///
/// Locked out is the face that names the stranded value:
/// a control whose every entry is refused holds a value the same evaluation greys,
/// the repair has nowhere to walk it, and "SRT" on a computer that cannot send SRT
/// reads as a settled answer to a question nothing here can answer
/// (<c>docs/field-availability.md</c>).
/// </summary>
public sealed class NothingLeftTests
{
    /// <summary>Select over the entries named. Dash prefix marks a refused one: "-srt".</summary>
    private static FieldViewModel Select(string picked, params string[] entries)
    {
        var field = new Field
        {
            Key = "publish.publish_transport",
            Control = ControlKind.Select,
            Visible = true,
            Enabled = true,
            Value = new FieldValue { Text = picked },
        };

        foreach (var entry in entries)
        {
            field.Options.Add(new FieldOption
            {
                Value = entry.TrimStart('-'),
                Enabled = !entry.StartsWith('-'),
                Reason = entry.StartsWith('-') ? new Text { Code = TextCode.EngineToolingMissing } : null,
            });
        }

        var view = new FieldViewModel(field.Key, (_, _) => { });
        view.Apply(field, Vocabulary.Empty);

        return view;
    }

    [Fact]
    public void AControlWithEveryEntryRefusedNamesNoValue()
    {
        var transport = Select("srt", "-srt", "-rtsp", "-webrtc");

        Assert.True(transport.HasNothingLeft);
        Assert.Equal(Fields.NothingLeft, transport.PickedLabel);
        Assert.DoesNotContain("SRT", transport.PickedLabel);
    }

    [Fact]
    public void AControlWithAnEntryLeftNamesThePick()
    {
        var transport = Select("srt", "srt", "-rtsp", "-webrtc");

        Assert.False(transport.HasNothingLeft);
        Assert.Equal("SRT", transport.PickedLabel);
    }

    /// <summary>
    /// The blocking line says which control is empty and what that costs.
    /// A reader meeting "Nothing available" on a chip opens the step and finds the sentence beside the list,
    /// so it names the thing the control decides rather than the control.
    /// </summary>
    [Theory]
    [InlineData("publish.capture", "capture a screen")]
    [InlineData("publish.format", "encode the picture")]
    [InlineData("publish.encoder", "produce this format")]
    [InlineData("publish.publish_transport", "carry the stream")]
    [InlineData("viewer.tile_watch_transport", "receive a stream")]
    [InlineData("viewer.render_chain", "draw a received picture")]
    public void TheStatementNamesWhatTheEmptyControlDecides(string key, string phrase)
    {
        var text = new Text { Code = TextCode.NothingLeftToPick };
        text.Args.Add(new TextArg { Name = TextArgName.Option, Id = key });

        var sentence = Statements.Of(text);

        Assert.Contains(phrase, sentence);
        Assert.Contains("Each entry says what it needs", sentence);
    }

    /// <summary>
    /// A refused pick beside a live entry is a draft the next resolve walks onto that entry.
    /// The face keeps naming what the settings hold, so the value and its replacement are both readable
    /// while the answer is in flight.
    /// </summary>
    [Fact]
    public void ARefusedPickBesideALiveEntryStillNamesItself()
    {
        var transport = Select("srt", "-srt", "rtsp");

        Assert.False(transport.HasNothingLeft);
        Assert.Equal("SRT", transport.PickedLabel);
    }
}
