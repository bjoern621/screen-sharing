using Avalonia.Controls.Documents;
using Google.Protobuf;
using ScreenShare.Api.V1;
using ScreenShare.App.Controls;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// One sentence carries both the words and where the picture goes,
/// so a path that knows nothing about Discord can hand it on whole.
/// The mark is machinery and reaches no screen: a line drawing no picture drops it.
/// </summary>
public sealed class AccountLineTests
{
    private static ByteString APicture => ByteString.CopyFromUtf8("png-bytes");

    [Fact]
    public void ThePictureStandsWhereTheSentenceMarksIt()
    {
        var line = new AccountLine
        {
            Sentence = $"Linked as {AccountLine.Mark}bjoern123. Following General in Guild.",
            Picture = APicture,
        };

        Assert.NotNull(line.Inlines);
        Assert.Equal(3, line.Inlines!.Count);
        Assert.Equal("Linked as ", Assert.IsType<Run>(line.Inlines[0]).Text);
        Assert.IsType<Avatar>(Assert.IsType<InlineUIContainer>(line.Inlines[1]).Child);
        Assert.Equal("bjoern123. Following General in Guild.", Assert.IsType<Run>(line.Inlines[2]).Text);
    }

    /// <summary>
    /// A picture that has yet to land leaves the sentence whole,
    /// so a reader meets the words rather than a character standing in for what is missing.
    /// </summary>
    [Fact]
    public void ASentenceWithNoPictureDropsTheMark()
    {
        var line = new AccountLine { Sentence = $"Linked as {AccountLine.Mark}bjoern123." };

        Assert.Equal("Linked as bjoern123.", Assert.IsType<Run>(line.Inlines![0]).Text);
        Assert.Single(line.Inlines);
    }

    [Fact]
    public void AnUnmarkedSentenceDrawsItsWordsAlone()
    {
        var line = new AccountLine
        {
            Sentence = "No Discord account is linked. Link one to follow a voice channel.",
            Picture = APicture,
        };

        Assert.Single(line.Inlines!);
        Assert.Equal(
            "No Discord account is linked. Link one to follow a voice channel.",
            Assert.IsType<Run>(line.Inlines![0]).Text);
    }

    /// <summary>The mark stands in front of the name, which is what puts the face beside it.</summary>
    [Fact]
    public void TheLinkedSentenceMarksTheNameItCarries()
    {
        var state = new DiscordState { Linked = true, AccountName = "bjoern123" };

        Assert.Equal($"Linked as {AccountLine.Mark}bjoern123", Copy.Links.Linked(state));
    }
}
