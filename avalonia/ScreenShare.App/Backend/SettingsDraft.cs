using Google.Protobuf;
using Google.Protobuf.Reflection;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// Writes one field of a settings draft, named as the form named it.
///
/// This is the whole of what the shell knows about <see cref="Settings"/>: that a <c>Field.key</c> is a group
/// of that message and a field in that group, and that a <c>FieldValue</c> fits it.
/// The write goes through the message descriptors rather than a switch over field names, which is what keeps
/// the shell free of the vocabulary - a field added to the contract is a control that appears and works, with
/// nothing here to edit (docs/ipc-api.md, "The rule").
/// </summary>
public static class SettingsDraft
{
    /// <summary>
    /// The character between a key's group and its field, as settings.proto spells the two: "relay.host",
    /// "publish.codec", "viewer.render_chain".
    /// </summary>
    public const char KeySeparator = '.';

    /// <summary>
    /// The settings group a preset is (<c>docs/presets.md</c>), which is the one group this shell has a
    /// reason to name.
    ///
    /// Read off the descriptor rather than typed out, so it is the same fact as the property a whole-group
    /// write assigns (<see cref="FormSession.WritePublish"/>).
    /// A spelling written here by hand would be a second name for one field, free to disagree with the keys
    /// the form sends the moment either moved.
    /// </summary>
    public static readonly string PublishGroup =
        Settings.Descriptor.FindFieldByNumber(Settings.PublishFieldNumber).Name;

    public static void Write(Settings draft, string key, FieldValue value)
    {
        Assert.NotNull(draft, "a settings write needs the draft it changes");
        Assert.NotNull(value, "a settings write needs a value to put in the field");

        var (group, descriptor) = Resolve(draft, key);
        descriptor.Accessor.SetValue(group, Fitted(descriptor, value, key));
    }

    /// <summary>
    /// Reads one field of a settings draft, named as the form named it.
    /// It is Write's inverse and exists for the same reason: a key addresses a message here, in one place,
    /// rather than at every caller that wants a value out of a draft.
    /// </summary>
    public static FieldValue Read(Settings draft, string key)
    {
        Assert.NotNull(draft, "a settings read needs the draft it reads");

        var (group, descriptor) = Resolve(draft, key);
        var raw = descriptor.Accessor.GetValue(group);
        return descriptor.FieldType switch
        {
            FieldType.String => new FieldValue { Text = (string)raw },
            FieldType.Int32 => new FieldValue { Number = (int)raw },
            FieldType.Int64 => new FieldValue { Number = (long)raw },
            FieldType.Bool => new FieldValue { Flag = (bool)raw },
            FieldType.Double => new FieldValue { Decimal = (double)raw },
            _ => Assert.Never<FieldValue>("a form field's settings type is one the contract carries", key, descriptor.FieldType),
        };
    }

    /// <summary>
    /// The group message a key addresses and the field in it.
    ///
    /// A group the draft arrived without is created rather than refused: the draft is the shell's own and a
    /// group it has not been given yet is one nothing has written to, so the write that is about to happen is
    /// what fills it.
    /// A key that names no group, or no field in one, is a contract both sides disagree about.
    /// </summary>
    private static (IMessage Group, FieldDescriptor Field) Resolve(Settings draft, string key)
    {
        var separator = key.IndexOf(KeySeparator);
        Assert.That(separator > 0 && separator < key.Length - 1,
            "a form field names a settings group and a field in it", key);

        var groupName = key[..separator];
        var fieldName = key[(separator + 1)..];

        var groupField = Settings.Descriptor.FindFieldByName(groupName);
        Assert.That(groupField is not null, "a form field names a settings group", key);

        if (groupField.Accessor.GetValue(draft) is not IMessage group)
        {
            group = (IMessage)groupField.MessageType.Parser.ParseFrom(System.Array.Empty<byte>());
            groupField.Accessor.SetValue(draft, group);
        }

        if (fieldName.IndexOf(IndexOpen) >= 0)
        {
            return Entry(group, fieldName, key);
        }

        var descriptor = groupField.MessageType.FindFieldByName(fieldName);
        Assert.That(descriptor is not null, "a form field names a field of the group it belongs to", key);
        return (group, descriptor);
    }

    /// <summary>
    /// The brackets around the index of a repeated field: <c>publish.audio_sources[2].gain</c> names the
    /// third entry's gain.
    /// </summary>
    private const char IndexOpen = '[';

    private const char IndexClose = ']';

    /// <summary>
    /// The entry of a repeated field a key addresses, and the field inside that entry.
    ///
    /// An entry the list does not have yet is appended on the way, up to the one past its end and no further:
    /// that entry is the row the form draws for a reader to grow the list by, so a write through its key is
    /// what adds it, and a write past it would leave a hole nothing chose.
    /// The shell decides none of that - it walks the address the form gave it, exactly as it does for a plain
    /// field.
    /// </summary>
    private static (IMessage Group, FieldDescriptor Field) Entry(IMessage group, string path, string key)
    {
        var open = path.IndexOf(IndexOpen);
        var close = path.IndexOf(IndexClose);
        Assert.That(close > open + 1 && close + 1 < path.Length && path[close + 1] == KeySeparator,
            "an indexed form field names a list, an entry of it and a field in that entry", key);

        var listName = path[..open];
        var fieldName = path[(close + 2)..];
        Assert.That(int.TryParse(path[(open + 1)..close], out var index) && index >= 0,
            "an indexed form field names which entry of the list it edits", key);

        var listField = group.Descriptor.FindFieldByName(listName);
        Assert.That(listField is not null && listField.IsRepeated && listField.MessageType is not null,
            "a form field names a repeated message field of the group it belongs to", key);

        var list = (System.Collections.IList)listField.Accessor.GetValue(group);
        Assert.That(index <= list.Count, "a write reaches an entry the list has, or the one past its end", key, list.Count);
        if (index == list.Count)
        {
            list.Add(listField.MessageType.Parser.ParseFrom(System.Array.Empty<byte>()));
        }

        var entry = list[index] as IMessage;
        Assert.That(entry is not null, "an entry of a repeated message field is a message", key);

        var descriptor = listField.MessageType.FindFieldByName(fieldName);
        Assert.That(descriptor is not null, "a form field names a field of the entry it edits", key);
        return (entry, descriptor);
    }

    /// <summary>
    /// The value in the type the settings field holds.
    /// A mismatch between the two is a broken contract rather than a condition to survive: the backend chose
    /// the control kind from the field's own type, so a number arriving for a string field means the two ends
    /// disagree about the message.
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
