using System.IO;
using Avalonia.Media;
using Avalonia.Media.Imaging;
using Google.Protobuf;

namespace ScreenShare.App.Controls;

/// <summary>
/// Turns the bytes of a picture into one the toolkit draws.
///
/// Keyed by the bytes themselves, so the same picture decodes once however many rows carry it and
/// however often a pass rebuilds them.
/// Content and not state:
/// one reading of these bytes is every reading of them,
/// so a hit is what a decode would have produced.
/// </summary>
internal static class Pictures
{
    /// <summary>
    /// How many decoded pictures are kept, a handful of people in a voice channel plus room.
    /// Past it the lot goes, which costs the next pass a decode.
    /// </summary>
    private const int Held = 64;

    private static readonly Dictionary<ByteString, IImage> Decoded = [];

    /// <summary>
    /// The picture those bytes draw, null for no bytes and for bytes that are no picture.
    /// </summary>
    public static IImage? Of(ByteString? picture)
    {
        if (picture is null || picture.Length == 0)
        {
            return null;
        }

        lock (Decoded)
        {
            if (Decoded.TryGetValue(picture, out var held))
            {
                return held;
            }

            IImage drawn;
            try
            {
                drawn = new Bitmap(new MemoryStream(picture.ToByteArray()));
            }
            catch (Exception)
            {
                // Whatever the CDN answered was not an image, which is a row without a picture and
                // not a broken shell. Nothing is held, so a later read of the same address decodes again.
                return null;
            }

            if (Decoded.Count >= Held)
            {
                // Dropped rather than disposed: a bitmap a visible row still binds outlives the entry.
                Decoded.Clear();
            }
            Decoded[picture] = drawn;
            return drawn;
        }
    }
}
