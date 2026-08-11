using System.Net.Sockets;
using System.Runtime.InteropServices;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// The other half of the frame channel on a platform whose handles are file descriptors.
///
/// <b>A descriptor is not a number that can be sent.</b> It indexes one process's own table, so
/// the value naming a frame in the backend names something else here, or nothing at all. The
/// kernel's way to move one is <c>SCM_RIGHTS</c> over a Unix socket, which installs a
/// descriptor of this process's own naming the same memory - which is why a dmabuf pool
/// announces a socket path where a shared-texture pool announces a number
/// (<c>api/proto/screenshare/v1/frame.proto</c>, <c>FramePool.fd_socket</c>).
///
/// <b>It reads and does not import.</b> What the descriptors become on the GPU belongs to the
/// control that draws; this is the transport, and it sits beside the channel that named the
/// socket.
///
/// The backend answers every connection with the same set for as long as the pool lives, so
/// reading is repeatable: a generation that is re-imported reads the descriptors again rather
/// than depending on a handshake that happened once.
/// </summary>
internal static class FrameDescriptors
{
    /// <summary>
    /// Receives one descriptor per slot, in index order.
    ///
    /// The read runs off the UI thread because it is a socket round trip with another process,
    /// and it is bounded by the caller's cancellation: a backend that died mid-pool leaves a
    /// socket that accepts and never answers, which would otherwise be a tile waiting forever.
    /// </summary>
    public static Task<int[]> ReceiveAsync(string socketPath, int slots, CancellationToken cancellation)
    {
        Assert.That(socketPath.Length > 0, "a descriptor pool names the socket it is lent over");
        Assert.That(slots > 0, "a pool lends at least one slot", slots);

        return Task.Run(() => Receive(socketPath, slots, cancellation), cancellation);
    }

    private static int[] Receive(string socketPath, int slots, CancellationToken cancellation)
    {
        using var socket = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        socket.Connect(new UnixDomainSocketEndPoint(socketPath));

        var descriptors = new int[slots];
        var received = 0;
        try
        {
            for (; received < slots; received++)
            {
                var (index, descriptor) = ReceiveOne(socket, cancellation);
                if (index != received)
                {
                    Close(descriptor);
                    throw new BackendUnavailableException(
                        $"The backend lent slot {index} where slot {received} was expected.");
                }
                descriptors[received] = descriptor;
            }
        }
        catch
        {
            // Every descriptor already received is this process's own, and a caller that never
            // got the set has nothing to close them with.
            Close(descriptors, received);
            throw;
        }
        return descriptors;
    }

    /// <summary>
    /// One message: the slot's index as the payload and the slot's descriptor as the right that
    /// rides with it. The two travel together so a descriptor cannot be paired with the wrong
    /// slot, whatever order the reads happen in.
    /// </summary>
    private static unsafe (int Index, int Descriptor) ReceiveOne(Socket socket, CancellationToken cancellation)
    {
        var payload = stackalloc byte[1];
        var control = stackalloc byte[ControlSpace];

        var vector = new IoVector { Base = (IntPtr)payload, Length = (IntPtr)1 };
        var message = new MessageHeader
        {
            Vectors = (IntPtr)(&vector),
            VectorCount = (IntPtr)1,
            Control = (IntPtr)control,
            ControlLength = (IntPtr)ControlSpace,
        };

        long read;
        while (true)
        {
            cancellation.ThrowIfCancellationRequested();
            // Polled rather than blocked in: the descriptor arrives on a socket the backend
            // may never answer on, and a blocking read there is a tile that waits for the rest
            // of the run.
            if (!socket.Poll(PollInterval, SelectMode.SelectRead))
            {
                continue;
            }
            read = recvmsg(socket.Handle, &message, 0);
            if (read >= 0)
            {
                break;
            }
            var error = Marshal.GetLastPInvokeError();
            if (error is not (EAGAIN or EINTR))
            {
                throw new BackendUnavailableException(
                    $"The frames' descriptors could not be read: error {error}.");
            }
        }

        if (read != 1)
        {
            throw new BackendUnavailableException("The backend closed the descriptor socket early.");
        }

        // One right per message, and it is a descriptor. Anything else is a backend speaking a
        // protocol this build does not know, which is worth saying rather than dereferencing.
        var header = (ControlHeader*)control;
        if ((long)message.ControlLength < ControlSpace || header->Level != SOL_SOCKET ||
            header->Type != SCM_RIGHTS)
        {
            throw new BackendUnavailableException("The backend sent a frame slot with no descriptor.");
        }

        return (payload[0], *(int*)(control + ControlDataOffset));
    }

    private static void Close(int[] descriptors, int count)
    {
        for (var i = 0; i < count; i++)
        {
            Close(descriptors[i]);
        }
    }

    private static void Close(int descriptor)
    {
        if (descriptor >= 0)
        {
            close(descriptor);
        }
    }

    /// <summary>
    /// Closes descriptors this process owns. Every one that was received is this process's own,
    /// so a pool that is dropped without closing them leaks a descriptor and pins the memory
    /// the backend has already freed.
    /// </summary>
    public static void Release(IReadOnlyList<int> descriptors)
    {
        foreach (var descriptor in descriptors)
        {
            Close(descriptor);
        }
    }

    /// <summary>How long one poll waits before the cancellation is looked at again.</summary>
    private const int PollInterval = 100_000;

    private const int SOL_SOCKET = 1;
    private const int SCM_RIGHTS = 1;
    private const int EAGAIN = 11;
    private const int EINTR = 4;

    /// <summary>
    /// The control buffer's shape, which is <c>CMSG_SPACE(sizeof(int))</c> written out.
    ///
    /// It is computed from the pointer size rather than pinned at the 64-bit numbers, because
    /// the header's first field is a <c>size_t</c> and its alignment is the same word: the
    /// layout the kernel writes differs between a 64-bit and a 32-bit build of this app.
    /// </summary>
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
    private static extern unsafe long recvmsg(IntPtr socket, MessageHeader* message, int flags);

    [DllImport("libc", SetLastError = true)]
    private static extern int close(int descriptor);
}
