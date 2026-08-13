using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// Conversions between a <see cref="FieldValue"/> and the string an option carries.
///
/// An option value is a string whatever the settings field's type, since one bindable list has one element
/// type.
/// Both directions live here rather than per control, so a select over a number cannot mark one entry and
/// write another.
/// </summary>
public static class FieldValues
{
    /// <summary>
    /// The value as an option carries it.
    /// Invariant formatting: the string is matched against option values the backend wrote, never shown.
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
    /// The kind comes from the value the form carried, so the answer takes the settings field's own type
    /// without this code naming the field.
    ///
    /// Text that will not parse is an Entwicklungsfehler: the backend wrote every option value into this
    /// field's own list.
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
