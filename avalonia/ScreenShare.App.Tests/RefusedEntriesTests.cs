using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A choice control lists what this combination allows and keeps the rest behind a disclosure.
///
/// What these lock out is a list that decides anything: which entries a combination rules out arrives in the
/// message, and all that is under test is where a shell puts them (docs/field-availability.md, "Where a greyed
/// entry sits").
/// </summary>
public sealed class RefusedEntriesTests
{
    /// <summary>A select over the entries named, the ones prefixed with a dash being the refused ones.</summary>
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

        Assert.Equal(["portal"], capture.Shown.Select(option => option.Value));
        Assert.True(capture.HasRefused);
        Assert.Equal("2 options", capture.RefusedCount);
    }

    [Fact]
    public void RevealingListsTheRefusedEntriesUnderTheOnesThatCanBePicked()
    {
        var capture = Select("portal", "portal", "-kmsgrab", "-ximage");

        capture.RevealCommand.Execute(null);

        Assert.Equal(["portal", "kmsgrab", "ximage"], capture.Shown.Select(option => option.Value));
        Assert.True(capture.RefusedShown);
    }

    /// <summary>Pressing it twice is a round trip, which is what the toggle's verb promises.</summary>
    [Fact]
    public void HidingThemAgainLeavesTheListAsItWas()
    {
        var capture = Select("portal", "portal", "-kmsgrab");
        var before = capture.Shown.ToList();

        capture.RevealCommand.Execute(null);
        capture.RevealCommand.Execute(null);

        Assert.Equal(before, capture.Shown);
        Assert.False(capture.RefusedShown);
    }

    /// <summary>
    /// A refused entry keeps the sentence naming what to change, which is the whole reason it is still on the
    /// list rather than gone from it.
    /// </summary>
    [Fact]
    public void ARevealedEntryStillStatesWhyItCannotBePicked()
    {
        var capture = Select("portal", "portal", "-kmsgrab");

        capture.RevealCommand.Execute(null);

        var refused = capture.Shown.Single(option => option.Value == "kmsgrab");
        Assert.False(refused.IsEnabled);
        Assert.True(refused.HasReason);
    }

    /// <summary>
    /// A flyout holds nothing but its items, so the disclosure is the last of them and reads as a state: the
    /// tick says whether the entries are listed (docs/design-language.md, "Menus").
    /// </summary>
    [Fact]
    public void ADropdownCarriesTheDisclosureAsItsLastRow()
    {
        var capture = Select("portal", "portal", "-kmsgrab");

        var reveal = capture.MenuRows.Last();

        Assert.True(reveal.IsReveal);
        Assert.True(reveal.IsEnabled);
        Assert.False(reveal.IsSelected);
        Assert.Equal(Fields.RefusedTitle, reveal.Label);
        Assert.Equal("1 option", reveal.Note);
        Assert.Equal(["portal"], capture.MenuRows.Where(row => !row.IsReveal).Select(row => row.Value));
    }

    [Fact]
    public void TheDisclosureReadsWhetherTheEntriesAreListed()
    {
        var capture = Select("portal", "portal", "-kmsgrab");

        capture.MenuRows.Last().Choose.Execute(null);

        Assert.True(capture.MenuRows.Last().IsSelected);
        Assert.Equal(["portal", "kmsgrab"], capture.MenuRows.Where(row => !row.IsReveal).Select(row => row.Value));
    }

    [Fact]
    public void AControlWithNothingRefusedOffersNothingToReveal()
    {
        var capture = Select("portal", "portal", "kmsgrab");

        Assert.False(capture.HasRefused);
        Assert.Equal("", capture.RefusedCount);
        Assert.Equal(["portal", "kmsgrab"], capture.Shown.Select(option => option.Value));
        Assert.Equal(capture.Shown, capture.MenuRows);
    }

    /// <summary>
    /// A pass is the backend's answer and the reveal is the reader's, so a fresh form leaves an opened list
    /// open (docs/development-principles.md, "One render function per component").
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
        var before = capture.Shown.ToList();

        capture.Apply(field, Vocabulary.Empty);

        Assert.True(capture.RefusedShown);
        Assert.Equal(before, capture.Shown);
    }
}
