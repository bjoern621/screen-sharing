using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A number-select entry outside its own range.
///
/// The burst ceiling is legal at zero, meaning bounded by nothing, and legal again from the target it bursts
/// above. A range runs from one end to the other and cannot hold that pair, so the backend sends the band as
/// the range and the zero as an entry
/// (api/proto/screenshare/v1/form.proto, CONTROL_KIND_NUMBER_SELECT).
///
/// A spinner would coerce such a value into its own floor and write the floor back, which is the one answer
/// the entry exists to reach. So the entry is drawn as itself while it is held, and the spinner returns with
/// the next value inside the band.
/// </summary>
public sealed class OutsideBandEntryTests
{
    private static FieldViewModel Ceiling(long held, long min, long max)
    {
        var field = new FieldViewModel("publish.maxrate_mbps", (_, _) => { });
        field.Apply(
            new Field
            {
                Key = "publish.maxrate_mbps",
                Control = ControlKind.NumberSelect,
                Visible = true,
                Enabled = true,
                Range = new NumericRange { Min = min, Max = max, Step = 1 },
                Value = new FieldValue { Number = held },
                Options =
                {
                    new FieldOption { Value = "0", Enabled = true },
                    new FieldOption { Value = "40", Enabled = true },
                    new FieldOption { Value = "80", Enabled = true },
                },
            },
            Vocabulary.Empty);
        return field;
    }

    /// <summary>A ceiling of zero under a band starting at the target is the entry, not a number to sweep.</summary>
    [Fact]
    public void AValueUnderTheBandIsDrawnAsItsEntry()
    {
        var field = Ceiling(held: 0, min: 40, max: 2147);

        Assert.True(field.IsEntryOutsideBand);
        Assert.False(field.IsNumberBox);
    }

    /// <summary>A ceiling inside the band is the ordinary number-select the frame rate is.</summary>
    [Fact]
    public void AValueInsideTheBandKeepsTheNumber()
    {
        var field = Ceiling(held: 80, min: 40, max: 2147);

        Assert.False(field.IsEntryOutsideBand);
        Assert.True(field.IsNumberBox);
    }

    /// <summary>
    /// The entry a reader sees is the one the backend sent, named by this shell.
    /// A value outside the band that the ladder does not carry is no entry at all, and the number stands.
    /// </summary>
    [Fact]
    public void AValueOutsideTheBandAndOffTheLadderKeepsTheNumber()
    {
        var field = Ceiling(held: 7, min: 40, max: 2147);

        Assert.False(field.IsEntryOutsideBand);
        Assert.True(field.IsNumberSelect);
    }

    /// <summary>
    /// The whole point of the entry: what is held stays held.
    /// A control that reported the band's floor here would have replaced an uncapped burst with a capped one
    /// without anybody asking.
    /// </summary>
    [Fact]
    public void TheHeldValueIsNotPulledIntoTheBand()
    {
        var field = Ceiling(held: 0, min: 40, max: 2147);

        Assert.Equal(0, field.Number);
        Assert.Equal("0", field.Options.Single(option => option.IsSelected).Value);
    }
}
