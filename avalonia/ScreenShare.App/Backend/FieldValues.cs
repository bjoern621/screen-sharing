using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// The two conversions between a <see cref="FieldValue"/> and the string an option carries.
///
/// An option value is a string whatever the settings field's type is, because one list has one element type
/// and a list of four shapes would be a list nothing can bind.
/// That leaves exactly two places where the string and the typed value have to meet - marking which entry is
/// picked, and reporting which entry was picked - and both are here rather than restated per control, so a
/// select over a number cannot mark one entry and write another.
/// </summary>
public static class FieldValues
{
    /// <summary>
    /// The value as an option carries it.
    /// Invariant formatting, because the string is compared against option values the backend wrote, not
    /// shown to anyone.
    /// </summary>
    public static string AsText(FieldValue? value) => value?.KindCase switch
    {
        FieldValue.KindOneofCase.Text => value.Text,
        FieldValue.KindOneofCase.Number => value.Number.ToString(CultureInfo.InvariantCulture),
        FieldValue.KindOneofCase.Decimal => value.Decimal.ToString(CultureInfo.InvariantCulture),
        FieldValue.KindOneofCase.Flag => value.Flag ? "true" : "false",
        FieldValue.KindOneofCase.None or null => "",
        _ => Assert.Never<string>("a field value carries a kind the contract defines", (int)value.KindCase),
    };

    /// <summary>
    /// The option's string back in the shape the field holds it.
    /// The kind comes from the value the form carried, so the answer matches the settings field's own type
    /// without this code knowing which field it is.
    ///
    /// A string that will not parse is a broken contract and not a condition to survive: every option value
    /// was written by the backend, into the list for this field.
    /// </summary>
    public static FieldValue Of(FieldValue.KindOneofCase kind, string text) => kind switch
    {
        FieldValue.KindOneofCase.Text => new FieldValue { Text = text },
        FieldValue.KindOneofCase.Number => new FieldValue { Number = long.Parse(text, CultureInfo.InvariantCulture) },
        FieldValue.KindOneofCase.Decimal => new FieldValue { Decimal = double.Parse(text, CultureInfo.InvariantCulture) },
        FieldValue.KindOneofCase.Flag => new FieldValue { Flag = bool.Parse(text) },
        _ => Assert.Never<FieldValue>("an option belongs to a field whose value has a kind", (int)kind),
    };
}
