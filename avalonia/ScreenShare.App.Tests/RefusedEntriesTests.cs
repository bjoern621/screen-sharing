using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A choice control lists what this combination allows and keeps the rest behind a disclosure.
/// Locked out is a list deciding anything: which entries a combination rules out arrives in the message,
/// and under test is only where a shell puts them (<c>docs/field-availability.md</c>, "Where a greyed entry sits").
/// </summary>
public sealed class RefusedEntriesTests
{
    /// <summary>Select over the entries named. Dash prefix marks a refused one: "-kmsgrab".</summary>
    private static FieldViewModel Select(string picked, params string[] entries)
    {
        var field = new Field
        {
            Key = "publish.capture",
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
                Reason = entry.StartsWith('-') ? new Text { Code = TextCode.CodecNotImplemented } : null,
            });
        }

        var view = new FieldViewModel(field.Key, (_, _) => { });
        view.Apply(field, Vocabulary.Empty);

        return view;
    }

    [Fact]
    public void AListDrawsWhatCanBePickedAndCountsTheRest()
    {
        var capture = Select("portal", "portal", "-kmsgrab", "-ximage");

        Assert.Equal(["portal"], capture.Offered.Select(option => option.Value));
        Assert.Equal(["kmsgrab", "ximage"], capture.Refused.Select(option => option.Value));
        Assert.True(capture.HasRefused);
        Assert.Equal("2 options", capture.RefusedCount);
        Assert.False(capture.RefusedShown);
    }

    /// <summary>
    /// The disclosure sits over what it covers, so the press that opens the list is where the press closing it is.
    /// A list that grew above it would walk out from under the pointer.
    /// </summary>
    [Fact]
    public void RevealingLeavesTheEntriesAboveTheDisclosureWhereTheyWere()
    {
        var capture = Select("portal", "portal", "-kmsgrab", "-ximage");
        var above = capture.Offered.ToList();
        var below = capture.Refused.ToList();

        capture.RevealCommand.Execute(null);

        Assert.Equal(above, capture.Offered);
        Assert.Equal(below, capture.Refused);
        Assert.True(capture.RefusedShown);
    }

    /// <summary>Pressing it twice is a round trip, what the toggle's verb promises.</summary>
    [Fact]
    public void HidingThemAgainLeavesTheListAsItWas()
    {
        var capture = Select("portal", "portal", "-kmsgrab");
        var offered = capture.Offered.ToList();
        var refused = capture.Refused.ToList();

        capture.RevealCommand.Execute(null);
        capture.RevealCommand.Execute(null);

        Assert.Equal(offered, capture.Offered);
        Assert.Equal(refused, capture.Refused);
        Assert.False(capture.RefusedShown);
    }

    /// <summary>
    /// A refused entry keeps the sentence naming what to change,
    /// the reason it stays on the list rather than going from it.
    /// </summary>
    [Fact]
    public void ARefusedEntryStatesWhyItCannotBePicked()
    {
        var capture = Select("portal", "portal", "-kmsgrab");

        var refused = capture.Refused.Single(option => option.Value == "kmsgrab");

        Assert.False(refused.IsEnabled);
        Assert.True(refused.HasReason);
    }

    /// <summary>
    /// A flyout holds nothing but its items, so the disclosure is a row among them and reads as a state:
    /// the tick says whether the entries are listed (<c>docs/design-language.md</c>, "Menus").
    /// It sits under the entries that can be picked, where the card list draws it.
    /// </summary>
    [Fact]
    public void ADropdownCarriesTheDisclosureUnderTheEntriesThatCanBePicked()
    {
        var capture = Select("portal", "portal", "-kmsgrab");

        var reveal = capture.MenuRows.Single(row => row.IsReveal);

        Assert.Equal(capture.Offered.Count, capture.MenuRows.IndexOf(reveal));
        Assert.True(reveal.IsEnabled);
        Assert.False(reveal.IsSelected);
        Assert.Equal(Fields.RefusedTitle, reveal.Label);
        Assert.Equal("1 option", reveal.Note);
        Assert.Equal(["portal"], capture.MenuRows.Where(row => !row.IsReveal).Select(row => row.Value));
    }

    /// <summary>Opened, the entries arrive under the row that opened them and the row keeps its place.</summary>
    [Fact]
    public void TheDisclosureKeepsItsRowWhenTheEntriesAreListed()
    {
        var capture = Select("portal", "portal", "-kmsgrab");

        capture.MenuRows[1].Choose.Execute(null);

        Assert.True(capture.MenuRows[1].IsReveal);
        Assert.True(capture.MenuRows[1].IsSelected);
        Assert.Equal(["portal", "kmsgrab"], capture.MenuRows.Where(row => !row.IsReveal).Select(row => row.Value));
    }

    [Fact]
    public void AControlWithNothingRefusedOffersNothingToReveal()
    {
        var capture = Select("portal", "portal", "kmsgrab");

        Assert.False(capture.HasRefused);
        Assert.Equal("", capture.RefusedCount);
        Assert.Equal(["portal", "kmsgrab"], capture.Offered.Select(option => option.Value));
        Assert.Empty(capture.Refused);
        Assert.Equal(capture.Offered, capture.MenuRows);
    }

    /// <summary>
    /// A pass is the backend's answer and the reveal the reader's, so a fresh form leaves an opened list open
    /// (<c>docs/development-principles.md</c>, "One render function per component").
    /// </summary>
    [Fact]
    public void AFurtherPassKeepsTheListTheReaderOpened()
    {
        var field = new Field
        {
            Key = "publish.capture",
            Control = ControlKind.Select,
            Visible = true,
            Enabled = true,
            Value = new FieldValue { Text = "portal" },
            Options =
            {
                new FieldOption { Value = "portal", Enabled = true },
                new FieldOption { Value = "kmsgrab", Enabled = false, Reason = new Text { Code = TextCode.CodecNotImplemented } },
            },
        };

        var capture = new FieldViewModel(field.Key, (_, _) => { });
        capture.Apply(field, Vocabulary.Empty);
        capture.RevealCommand.Execute(null);
        var rows = capture.MenuRows.ToList();

        capture.Apply(field, Vocabulary.Empty);

        Assert.True(capture.RefusedShown);
        Assert.Equal(rows, capture.MenuRows);
    }
}
