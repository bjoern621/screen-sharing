using System.Net.Sockets;
using System.Runtime.InteropServices;
using ScreenShare.App.Backend;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The consumer's half of a descriptor pool.
///
/// What this locks out is the one thing about it that fails silently: the shape of the kernel's
/// control message. A descriptor arrives beside the payload rather than in it, in a header whose
/// fields are pointer-sized and pointer-aligned, and a reader that lays that out wrong takes a
/// number out of the padding and imports whatever file it happens to name. The symptom is a tile
/// drawing noise on one machine and nothing on another, which is why the layout is asserted here
/// against descriptors of a known size.
///
/// The backend that would send them is not needed and is not run: what is under test is the
/// reader, so the sender is this file, and it writes exactly what
/// <c>internal/receive/descriptors_linux.go</c> writes.
/// </summary>
public sealed class FrameDescriptorsTests
{
    /// <summary>How many slots a lent pool holds, which is what the backend opens one with.</summary>
    private const int Slots = 3;

    /// <summary>The size the sent files are written to, so a received descriptor can be told apart from any other.</summary>
    private const int FileBytes = 4321;

    [Fact]
    public async Task ADescriptorPerSlotArrivesAndNamesTheMemoryItWasSentFor()
    {
        if (!OperatingSystem.IsLinux())
        {
            // Rights over a Unix socket are the Linux handle kind's transport, and no other
            // platform's pool announces a socket at all.
            return;
        }

        using var lent = new LentFiles(Slots, FileBytes);
        await using var sender = Sender.Listening(lent);

        var descriptors = await FrameDescriptors.ReceiveAsync(sender.Address, Slots, CancellationToken.None);

        Assert.Equal(Slots, descriptors.Length);
        for (var slot = 0; slot < Slots; slot++)
        {
            Assert.True(descriptors[slot] > 2, $"slot {slot} arrived as descriptor {descriptors[slot]}");
            Assert.Equal(FileBytes, Size(descriptors[slot]));
        }
        FrameDescriptors.Release(descriptors);
    }

    [Fact]
    public async Task ASetThatStopsPartWayThroughIsRefused()
    {
        if (!OperatingSystem.IsLinux())
        {
            return;
        }

        using var lent = new LentFiles(Slots - 1, FileBytes);
        await using var sender = Sender.Listening(lent);

        // A pool of three slots whose socket answers with two is a backend that died mid-pool,
        // and a reader that waited for the third would be a tile that never draws and never says
        // why.
        await Assert.ThrowsAsync<BackendUnavailableException>(
            () => FrameDescriptors.ReceiveAsync(sender.Address, Slots, CancellationToken.None));
    }

    /// <summary>The descriptors a test lends: ordinary files of a known size, which a reader can size back.</summary>
    private sealed class LentFiles : IDisposable
    {
        private readonly string _directory;
        private readonly List<FileStream> _files = [];

        public LentFiles(int count, int bytes)
        {
            _directory = Directory.CreateTempSubdirectory("screenshare-descriptors-").FullName;
            for (var i = 0; i < count; i++)
            {
                var file = File.Create(Path.Combine(_directory, $"slot-{i}"));
                file.Write(new byte[bytes]);
                file.Flush();
                _files.Add(file);
            }
        }

        public int Count => _files.Count;

        public int Descriptor(int slot) => (int)_files[slot].SafeFileHandle.DangerousGetHandle();

        public void Dispose()
        {
            foreach (var file in _files)
            {
                file.Dispose();
            }
            Directory.Delete(_directory, recursive: true);
        }
    }

    /// <summary>
    /// The backend's half: a socket that answers a connection with one message per slot, the
    /// slot's index as the payload and the slot's descriptor as the right beside it.
    /// </summary>
    private sealed class Sender : IAsyncDisposable
    {
        private readonly Socket _listener;
        private readonly Task _serving;
        private readonly string _directory;

        private Sender(Socket listener, string directory, LentFiles lent)
        {
            _listener = listener;
            _directory = directory;
            _serving = Task.Run(() => Serve(lent));
        }

        public static Sender Listening(LentFiles lent)
        {
            var directory = Directory.CreateTempSubdirectory("screenshare-pool-").FullName;
            var path = Path.Combine(directory, "pool.sock");

            var listener = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
            listener.Bind(new UnixDomainSocketEndPoint(path));
            listener.Listen(1);
            return new Sender(listener, directory, lent);
        }

        public string Address => ((UnixDomainSocketEndPoint)_listener.LocalEndPoint!).ToString()!;

        private void Serve(LentFiles lent)
        {
            using var connection = _listener.Accept();
            for (var slot = 0; slot < lent.Count; slot++)
            {
                Send(connection.Handle, (byte)slot, lent.Descriptor(slot));
            }
        }

        public async ValueTask DisposeAsync()
        {
            _listener.Dispose();
            try
            {
                await _serving;
            }
            catch (Exception)
            {
                // The accept was cancelled by the dispose, which is how a test that never
                // connected ends.
            }
            Directory.Delete(_directory, recursive: true);
        }
    }

    private static unsafe void Send(IntPtr socket, byte payload, int descriptor)
    {
        var control = stackalloc byte[ControlSpace];
        new Span<byte>(control, ControlSpace).Clear();

        var header = (ControlHeader*)control;
        header->Length = (IntPtr)(ControlDataOffset + sizeof(int));
        header->Level = SOL_SOCKET;
        header->Type = SCM_RIGHTS;
        *(int*)(control + ControlDataOffset) = descriptor;

        var byteToSend = payload;
        var vector = new IoVector { Base = (IntPtr)(&byteToSend), Length = (IntPtr)1 };
        var message = new MessageHeader
        {
            Vectors = (IntPtr)(&vector),
            VectorCount = (IntPtr)1,
            Control = (IntPtr)control,
            ControlLength = (IntPtr)ControlSpace,
        };

        var sent = sendmsg(socket, &message, 0);
        Assert.Equal(1, sent);
    }

    private static long Size(int descriptor) => lseek(descriptor, 0, SEEK_END);

    private const int SOL_SOCKET = 1;
    private const int SCM_RIGHTS = 1;
    private const int SEEK_END = 2;

    private static readonly int ControlDataOffset = Align(IntPtr.Size + sizeof(int) + sizeof(int));
    private static readonly int ControlSpace = ControlDataOffset + Align(sizeof(int));

    private static int Align(int length) => (length + IntPtr.Size - 1) & ~(IntPtr.Size - 1);

    [StructLayout(LayoutKind.Sequential)]
    private struct IoVector
    {
        public IntPtr Base;
        public IntPtr Length;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct MessageHeader
    {
        public IntPtr Name;
        public uint NameLength;
        public IntPtr Vectors;
        public IntPtr VectorCount;
        public IntPtr Control;
        public IntPtr ControlLength;
        public int Flags;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct ControlHeader
    {
        public IntPtr Length;
        public int Level;
        public int Type;
    }

    [DllImport("libc", SetLastError = true)]
    private static extern unsafe long sendmsg(IntPtr socket, MessageHeader* message, int flags);

    [DllImport("libc", SetLastError = true)]
    private static extern long lseek(int descriptor, long offset, int whence);
}
