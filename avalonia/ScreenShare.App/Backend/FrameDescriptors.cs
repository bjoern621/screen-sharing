using System.Net.Sockets;
using System.Runtime.InteropServices;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// The frame channel's other half on a platform whose handles are file descriptors.
///
/// <b>A descriptor is not a number that can be sent.</b> It indexes one process's own table, so the value
/// naming a frame in the backend names something else here, or nothing at all.
/// <c>SCM_RIGHTS</c> over a Unix socket is the kernel's way to move one, installing an entry in this process's
/// own table for the same memory, which is why a dmabuf pool announces a socket path where a shared-texture
/// pool announces a number (<c>api/proto/screenshare/v1/frame.proto</c>,
/// <c>FramePool.fd_socket</c>).
///
/// <b>It reads and does not import.</b> What the descriptors become on the GPU belongs to the control that
/// draws, and this is the transport, beside the channel that named the socket.
///
/// The backend answers every connection with the same set for as long as the pool lives, so a re-imported
/// generation reads the descriptors again rather than depending on a handshake that happened once.
/// </summary>
internal static class FrameDescriptors
{
    /// <summary>
    /// One descriptor per slot, in index order.
    ///
    /// Off the UI thread because it is a socket round trip with another process, and bounded by the caller's
    /// cancellation: a backend that died mid-pool leaves a socket that accepts and never answers, which is
    /// otherwise a tile waiting for the rest of the run.
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
            // Descriptors already received are this process's own, and a caller that never got the array has
            // nothing to close them with.
            Close(descriptors, received);
            throw;
        }
        return descriptors;
    }

    /// <summary>
    /// One message: the slot index as the payload, the slot's descriptor as the right riding with it.
    /// Paired in one message so no order of reads can attach a descriptor to the wrong slot.
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
            // Polled rather than blocked in: the backend may never answer on this socket, and a blocking read
            // there is a tile that waits for the rest of the run.
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

        // One right per message, and it is a descriptor.
        // Anything else is a backend speaking a protocol this build does not know, so it is reported rather
        // than dereferenced.
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
    /// Closes the descriptors this process owns.
    /// A pool dropped without this leaks a descriptor per slot and pins memory the backend has already freed.
    /// </summary>
    public static void Release(IReadOnlyList<int> descriptors)
    {
        foreach (var descriptor in descriptors)
        {
            Close(descriptor);
        }
    }

    /// <summary>Microseconds one poll waits before the cancellation is looked at again.</summary>
    private const int PollInterval = 100_000;

    private const int SOL_SOCKET = 1;
    private const int SCM_RIGHTS = 1;
    private const int EAGAIN = 11;
    private const int EINTR = 4;

    /// <summary>
    /// The control buffer's shape, <c>CMSG_SPACE(sizeof(int))</c> written out.
    ///
    /// Computed from the pointer size rather than pinned to the 64-bit numbers: the header's first field is a
    /// <c>size_t</c> and aligns to the same word, so the layout the kernel writes differs between a 64-bit and
    /// a 32-bit build of this app.
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
