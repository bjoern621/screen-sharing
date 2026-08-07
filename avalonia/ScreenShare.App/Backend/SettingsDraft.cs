using Google.Protobuf.Reflection;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// Writes one field of a settings draft, named as the form named it.
///
/// This is the whole of what the shell knows about <see cref="StreamSettings"/>: that a
/// <c>Field.key</c> is a field of that message, and that a <c>FieldValue</c> fits it. The
/// write goes through the message descriptor rather than a switch over field names, which
/// is what keeps the shell free of the vocabulary - a field added to the contract is a
/// control that appears and works, with nothing here to edit
/// (docs/ipc-api.md, "The rule").
/// </summary>
public static class SettingsDraft
{
    public static void Write(StreamSettings draft, string key, FieldValue value)
    {
        Assert.NotNull(draft, "a settings write needs the draft it changes");
        Assert.NotNull(value, "a settings write needs a value to put in the field");

        var descriptor = StreamSettings.Descriptor.FindFieldByName(key);
        Assert.NotNull(descriptor, "a form field names a settings field");

        descriptor.Accessor.SetValue(draft, Fitted(descriptor, value, key));
    }

    /// <summary>
    /// The value in the type the settings field holds. A mismatch between the two is a
    /// broken contract rather than a condition to survive: the backend chose the control
    /// kind from the field's own type, so a number arriving for a string field means the
    /// two ends disagree about the message.
    /// </summary>
    private static object Fitted(FieldDescriptor descriptor, FieldValue value, string key) => descriptor.FieldType switch
    {
        FieldType.String => value.Text,
        FieldType.Int32 => (int)value.Number,
        FieldType.Int64 => value.Number,
        FieldType.Bool => value.Flag,
        FieldType.Double => value.Decimal,
        _ => Assert.Never<object>("a form field's settings type is one the contract carries", key, descriptor.FieldType),
    };
}
