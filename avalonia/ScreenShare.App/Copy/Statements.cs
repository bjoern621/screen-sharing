using System.Globalization;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Copy;

/// <summary>
/// Turns one statement from the backend into a sentence.
///
/// The backend sends no prose.
/// It sends a code naming the fact it is stating and the identifiers the fact is about, capture <c>portal</c>,
/// codec <c>libx264</c>, engine <c>gstreamer</c>, and this is where that becomes something to read
/// (api/proto/screenshare/v1/text.proto).
///
/// Every sentence names the limit, which side has it, and the way out, which makes a greyed control useful
/// rather than merely honest.
/// Where the backend hands over an alternative the sentence uses it, and where it hands over none the sentence
/// stops rather than trailing off.
/// Identifiers reach the screen through <see cref="Words"/>, so a reader is told about an NVIDIA GPU rather than
/// about <c>nvenc</c>, and the identifier comes back only where the reader meets it again in a log or a command
/// line.
///
/// An unknown code renders as the code: a shell older than its backend will meet one, and a blank where a reason
/// belongs reads as a control greyed for nothing.
/// </summary>
public static class Statements
{
    /// <summary>
    /// Sentence for one statement, and the empty string for none.
    /// Absence is the normal case: most controls carry no reason and most entries no note.
    /// </summary>
    public static string Of(Text? text)
    {
        if (text is null || text.Code == TextCode.Unspecified)
        {
            return "";
        }

        var a = new Args(text);
        return text.Code switch
        {
            // Capture backends and monitors.

            TextCode.CaptureWrongOs =>
                $"{Words.Capture(a.Capture)} needs {Words.OperatingSystem(a.Os)}.",

            TextCode.CaptureWrongSession =>
                $"{Words.Capture(a.Capture)} needs an {Words.DisplayServer(a.Display)} session. "
                + "On Wayland it would see only the older windows, not the desktop. Use the screen picker instead.",

            // A note and not a refusal, so a fragment: it prints on the entry's own row, where a sentence would
            // crowd out the name.
            // What the privilege is for is the entry's paragraph (Descriptions.Capture).
            TextCode.CaptureNeedsGrant => "needs elevated privileges",

            TextCode.CaptureTakesNoMonitor => a.Capture switch
            {
                "kmsgrab" => "This capture takes everything the GPU is scanning out, not one screen.",
                "gdigrab" => "This capture takes the whole desktop as one picture: every monitor side by side.",
                "portal" => "The desktop's own picker chooses what is shared, so there is nothing to pick here.",
                _ => "This capture method chooses what it grabs itself.",
            },

            // Each branch names what the session does rather than what it lacks, so the absence of a picture reads
            // as how the machine works and not as a fault.
            TextCode.NoMonitorPreview => a.Display == "wayland"
                ? "Wayland reaches a screen only through the desktop's own picker, which asks "
                  + "every time, so there is no picture of one screen to show here. The picker "
                  + "shows what is being shared when the stream starts."
                : "This system hands the app whichever screen it likes rather than the one asked "
                  + "for, so there is no picture of a single screen to show here.",

            TextCode.MonitorNotEnumerated =>
                "Not connected. It stays selected, so what the stream would capture is still readable. "
                + "Pick a screen that is plugged in.",

            TextCode.ScaledFromSource =>
                $"from {Plain(a.Width)} × {Plain(a.Height)}",

            // Publish engines and the encoder probe.

            TextCode.EngineToolingMissing =>
                $"{Words.Engine(a.Engine)} is not installed, so nothing that runs on it can encode here. "
                + "Install it, or pick a capture method that runs the other one.",

            TextCode.EngineHasNoPublishSink =>
                $"{Words.Capture(a.Capture)} runs on {Words.Engine(a.Engine)}, which cannot send over "
                + $"{Words.Transport(a.Transport)}. Change the capture method to reach it.",

            TextCode.PublishSinkElementMissing =>
                $"Sending over {Words.Transport(a.Transport)} needs {a.Element}, which this "
                + $"{Words.Engine(a.Engine)} install does not carry. Install the plugin it ships in, or "
                + "pick another way out.",

            TextCode.EngineNotProbed =>
                $"Nothing on {Words.Engine(a.Engine)} has been tested on this machine, because "
                + $"{Lower(Of(a.Cause))} Until then nothing is greyed out for missing hardware, "
                + "so a choice this machine cannot run will fail at the start rather than here.",

            TextCode.ProbeNoDevice =>
                $"No {Words.Family(a.Family)} encoder was found on this machine. Either the hardware is not here, "
                + "or its driver does not offer encoding.",

            TextCode.ProbeNoBuild =>
                $"This install has no {a.Codec} encoder. It is a missing piece of software rather than missing "
                + "hardware, so another build or another package would bring it back.",

            TextCode.ProbeFailed =>
                $"{Words.Engine(a.Engine)} could not run {a.Codec} on this machine.",

            TextCode.CodecNotImplemented =>
                "Nothing in this app codes this format.",

            TextCode.NoEncoderForFormat =>
                $"Nothing on this machine codes {Words.Format(a.Format)}. Another format goes out through an "
                + "encoder that is here.",

            TextCode.EncoderCodesNoFormat => a.Formats.Count > 0
                ? $"{Words.Encoder(a.Encoder)} codes {Words.List(a.Formats.Select(Words.Format))}, "
                  + $"not {Words.Format(a.Format)}."
                : $"{Words.Encoder(a.Encoder)} codes no {Words.Format(a.Format)}.",

            // Carriage.

            TextCode.TransportCarriesNoFormat => Ways(
                $"{Words.Transport(a.Transport)} cannot carry {Words.Format(a.Format)} on {Words.Engine(a.Engine)}",
                a.Transports.Count > 0
                    ? $"send it over {Words.List(a.Transports.Select(Words.Transport))}"
                    : "",
                a.OtherEngine.Length > 0
                    ? $"keep {Words.Transport(a.Transport)} and pick a capture method that runs {Words.Engine(a.OtherEngine)}"
                    : ""),

            TextCode.LegCarriesNoAudioCodec => a.AudioCodecs.Count > 0
                ? $"{Words.Transport(a.Transport)} carries no {Words.AudioCodec(a.AudioCodec)} track on "
                  + $"{Words.Engine(a.Engine)}. It carries {Words.List(a.AudioCodecs.Select(Words.AudioCodec))}."
                : $"{Words.Transport(a.Transport)} carries no audio at all on {Words.Engine(a.Engine)}. "
                  + "Send over another protocol, or turn audio off.",

            TextCode.RenderChainElementMissing =>
                $"This machine has no {a.Element}, which {Words.RenderChain(a.Value)} needs. "
                + "Another route converts the frames instead.",

            TextCode.EngineHasNoAudioEncoder =>
                $"{Words.Engine(a.Engine)} has no {Words.AudioCodec(a.AudioCodec)} encoder. "
                + "Change the capture method to reach the other engine.",

            TextCode.NoViewerReceivesOver =>
                $"No player here opens {Words.Transport(a.Transport)}. "
                + $"Watching goes over {Words.List(a.Transports.Select(Words.Transport))}.",

            TextCode.RelayServesNoFormatOver => a.Transports.Count > 0
                ? $"The relay does not re-serve {Words.Format(a.Format)} over {Words.Transport(a.Transport)}, so a "
                  + $"player would connect and receive nothing. Watch over {Words.List(a.Transports.Select(Words.Transport))}."
                : $"The relay does not re-serve {Words.Format(a.Format)} over {Words.Transport(a.Transport)}, so a "
                  + "player would connect and receive nothing.",

            // Pixel format, colour and decoding.

            TextCode.CodecCodesNoRgb =>
                "This encoder cannot take the desktop's pixels directly. Only H.265 and VP9 have a mode for it, "
                + "and only on some encoders. Pick 4:4:4 for the next best thing.",

            TextCode.CodecCannotEncodeChroma =>
                $"This encoder cannot code {Words.Chroma(a.Chroma)}. Most hardware encoders do 4:2:0 and nothing "
                + "else; the CPU encoders reach the rest.",

            TextCode.RgbIsFullRange =>
                "RGB uses every code value by definition, so there is nothing to choose here.",

            TextCode.DecodesInHardware =>
                $"Viewers decode this on the GPU on {Words.List(a.DecodeFamilies.Select(Words.DecodeFamily), "and")}.",

            TextCode.DecodesOnCpu =>
                $"No GPU decodes {Words.Format(a.Format)} at {Words.Chroma(a.Chroma)}, so every viewer spends CPU on it"
                + (a.Decoder.Length > 0 ? $", through {a.Decoder}" : "")
                + ". That is a trade the publisher is entitled to make: it plays everywhere, and costs them.",

            TextCode.DecodesInHardwarePartly =>
                $"{Words.Chroma(a.Chroma)} {Words.Format(a.Format)} reaches a GPU on "
                + $"{Words.List(a.DecodeFamilies.Select(Words.DecodeFamily), "and")} and nowhere else. "
                + "Everyone else decodes it on the CPU, which still works and costs them cores.",

            // Frame memory and the GPU path.

            TextCode.PairHasNoDeviceMemory =>
                $"{Words.Capture(a.Capture)} and this encoder share no memory on {Words.Engine(a.Engine)}, "
                + "so the frames go through main memory whatever this is set to.",

            TextCode.PairConvertsOnDevice =>
                $"{Words.Capture(a.Capture)} and this encoder already convert on the GPU using the colour chosen here, "
                + $"so there is nothing to trade. \"{Words.Memory(a.Memory)}\" keeps both.",

            TextCode.PairTradesColour => Sentences(
                $"{Words.Capture(a.Capture)} and this encoder can share memory on {Words.Engine(a.Engine)}, but "
                + "nothing between them can convert the colour",
                Of(a.Cost),
                Of(a.Reach)),

            TextCode.ExactColourReach =>
                $"{Words.Capture(a.Capture)} on {Words.Engine(a.Engine)} shares the same screen and does convert, "
                + "so it keeps the GPU path and the colour chosen here.",

            TextCode.DevicePathHasNoScaler =>
                "On this path the frames never leave the GPU and nothing on it can resize them. "
                + $"Send at the source size, or set frames to \"{Words.Memory(a.Memory)}\".",

            TextCode.DrmMapUnusedOnDevice =>
                "The frames stay on the GPU, so nothing is copied back and no route is chosen.",

            // What carries the frames on each GPU path.

            TextCode.ImportGstPortalVaapi =>
                "The picker's frames are handed to the encoder directly, and the GPU converts them on the way.",

            TextCode.ImportGstD3D11Nvenc =>
                "The captured texture is converted on the GPU and handed straight to the NVIDIA encoder.",

            TextCode.ImportFfmpegKmsgrabVaapi =>
                "The scanout buffer is handed to the encoder as a GPU surface and converted there.",

            TextCode.ImportFfmpegDdagrabQsv =>
                "The captured texture is handed to Quick Sync as a GPU frame and converted there.",

            TextCode.ImportFfmpegDdagrabNvenc =>
                "The NVIDIA encoder reads the captured texture on its own device, with nothing converting in between.",

            TextCode.CostEncoderSignalsItsOwnColour =>
                $"The encoder converts the picture itself and sends {Words.Chroma(a.Chroma)} at "
                + $"{Words.ColorRange(a.ColorRange)} whatever is picked here.",

            // Rate control.

            TextCode.CqOnlyInConstantQuality =>
                "There is a quality target only when the encoder is holding quality. Switch to constant quality to set one.",

            TextCode.BitrateNotInMode =>
                $"{Words.Mode(a.Mode)} aims at no bandwidth figure. It spends whatever the picture costs.",

            // The pointer.

            TextCode.KmsgrabHasNoCursorPlane =>
                $"{Words.Capture(a.Capture)} reads the screen as it is scanned out, and the pointer is composed "
                + "over that at the last moment by the display hardware. There is nothing on that path to draw it in.",

            TextCode.CaptureHasNoCursorMetadata =>
                $"{Words.Capture(a.Capture)} hands over a picture and nothing else. Only the desktop portal reports "
                + "where the pointer is, separately from the frames.",

            TextCode.CursorMetadataLocalOnly =>
                "The position leaves the capture and reaches this machine, so the preview here draws it. "
                + "Nothing carries it over the relay, so people watching from elsewhere see no pointer.",

            TextCode.CursorMetadataNotCarried =>
                "Nothing here reads the pointer position this capture reports, so a stream sent this way would "
                + "arrive with no pointer at all.",

            TextCode.CqAboveCodecScale =>
                $"{a.Codec} counts quality to {a.CqMax} on {Words.Engine(a.Engine)}, so the slider stops there. "
                + "Each encoder has its own scale, and the same number is a different quality on each.",

            TextCode.BitrateAboveCodecLimit =>
                $"{a.Codec} refuses a target above {a.BitrateLimitMbps} Mbit/s on {Words.Engine(a.Engine)}. "
                + "It is the encoder's own limit, and an encode asking for more does not start.",

            TextCode.GopAboveCodecLimit =>
                $"{a.Codec} holds a keyframe interval of at most {a.GopLimitFrames} frames on {Words.Engine(a.Engine)}. "
                + "It is the encoder's own field, and an encode asking for a longer one does not start.",

            TextCode.DriverDefectWithholdsOption =>
                $"{a.Codec} codes this, and the graphics driver here does not survive it: on {a.GpuModel} "
                + "such a stream stops the share partway through and takes the picture down with it. "
                + "The other choices on this control run on it.",

            TextCode.MaxrateOnlyInConstrainedVbr =>
                "A ceiling belongs to the modes that let the rate move: capped variable bitrate, and constant quality. "
                + "The others hold the target itself.",

            TextCode.VbvOnlyInBoundedModes =>
                "The buffer only means something while the encoder is holding a bandwidth figure.",

            TextCode.NoCeilingInConstantQuality =>
                $"{a.Codec} holds a quality target and spends whatever the picture costs, with nothing above it to "
                + "stop at. Capped variable bitrate is the mode that bounds the rate on this encoder.",

            TextCode.VbvNeedsACeiling =>
                "The buffer is how long the stream may sit above the ceiling. Set a ceiling and it applies to that.",

            TextCode.BframesOffInMode =>
                $"{Words.Mode(a.Mode)} switches these off: they would save nothing here and still add delay.",

            TextCode.BframesOnlyOnFamilies =>
                $"Only the {Words.List(a.Families.Select(Words.Family), "and")} encoders take these.",

            TextCode.CodecTakesNoEffortLadder =>
                $"{a.Codec} has no such setting: how hard it works is that encoder's own to decide.",

            TextCode.EffortPinnedByMode =>
                $"{Words.Mode(a.Mode)} pins this to {Words.Effort(a.Effort)} for its low-delay tuning.",

            TextCode.CodecTakesNoTuneLadder =>
                $"{a.Codec} tunes for nothing picked here: it encodes the picture the same way whatever it contains.",

            TextCode.TunePinnedByMode =>
                $"{Words.Mode(a.Mode)} pins this to {Words.Tune(a.Tune)}, which is what the mode is for.",

            TextCode.AudioCodecNeedsSource =>
                "No audio is being sent, so there is nothing to compress.",

            TextCode.EngineTagsStandardRange =>
                $"{Words.Engine(a.Engine)} tags every stream standard range and cannot read what this screen is producing, "
                + $"so ten bits buys precision here and not high dynamic range. {Words.Engine(a.OtherEngine)} carries it.",

            TextCode.AudioEntryNeedsSource =>
                "Pick where this row records from first.",

            TextCode.AudioSourceHasOneDevice =>
                $"{Words.AudioSource(a.Audio)} has one device on this machine, so there is nothing to choose between.",

            TextCode.AudioDeviceNotEnumerated =>
                "not here right now",

            TextCode.AudioSourceUnservedByEngine =>
                $"{Words.Engine(a.Engine)} has nothing that opens {Words.AudioSource(a.Audio)} audio. "
                + $"Pick a capture method that runs {Words.Engine(a.OtherEngine)}.",

            TextCode.AudioSourceUnserved => a.Os switch
            {
                "windows" => "Windows records one program's own sound only through its process id, which nothing "
                    + "here can look up. Recording everything the machine plays reaches the same sound.",
                "darwin" => "macOS offers no system-audio input device, so neither encoder can record what it plays. "
                    + "A loopback driver is the usual workaround.",
                _ => $"{Words.OperatingSystem(a.Os)} offers nothing here that either encoder can record from.",
            },

            TextCode.AudioSourceServer => a.Os switch
            {
                "linux" => "through PulseAudio or PipeWire",
                "windows" => "through Windows audio",
                _ => "",
            },

            TextCode.AudioTrackCodedAt =>
                $"{Decimal(a.RateHz / 1000.0)} kHz · {Number(a.BitrateKbps)} kbit/s",

            // Where an engine departs from the mode.

            TextCode.Rav1ESizesNoRateBuffer =>
                "rav1e has no rate buffer, in any mode.",

            TextCode.GstRav1EncNoKeyframeInterval =>
                "The GStreamer rav1e element has no keyframe-interval setting, so its own default stands.",

            TextCode.GstNvencNoRateBuffer =>
                "The GStreamer NVIDIA elements have no rate-buffer setting.",

            TextCode.GstQsvNoRateBuffer =>
                "The GStreamer Quick Sync elements have no rate-buffer setting. It works on ffmpeg.",

            TextCode.QualityCeilingRequired =>
                $"{a.Codec} has no unbounded quality mode: it always codes toward a rate it stays under, so this is "
                + "the one figure it cannot be left without.",

            TextCode.FixedFunctionAbrDerivesCeiling =>
                "Hardware encoders always code against a ceiling, so this target is sent with twice itself as one. "
                + "The average is what the target holds.",

            TextCode.AmfCodesNoBframes =>
                "AMD's encoders are driven with look-ahead frames switched off, so a live stream pays none of their delay.",

            TextCode.FfmpegVaapiQualityIsTheDriversScale =>
                "How hard this encoder works is left to the graphics driver here, which counts it on a scale of its "
                + "own. A capture method that runs GStreamer offers the setting.",

            TextCode.VaapiCeilingBound =>
                $"On this encoder the ceiling cannot exceed {Number(a.MaxrateMbps)} Mbit/s against a "
                + $"{Decimal(a.BitrateMbps)} Mbit/s target. It sets the target as a percentage of the ceiling, "
                + "and half is as low as it goes.",

            // Codec capability gaps.

            TextCode.GapNvencAv1NoLosslessTune =>
                "NVIDIA's AV1 encoder has no lossless mode, unlike its H.264 and H.265 ones.",

            TextCode.GapGstVp9EncNoLossless =>
                "The GStreamer VP9 element has no lossless setting. It works on ffmpeg.",

            TextCode.GapVp8HasNoLossless =>
                "VP8 has no lossless mode at all. VP9 is the first of the two with one.",

            TextCode.GapGstAv1EncEightBitOnly =>
                "The GStreamer libaom element takes 8-bit input only. 10-bit works on ffmpeg.",

            TextCode.GapLibaomNoLosslessSwitch =>
                "Neither build exposes libaom's lossless switch.",

            TextCode.GapGstAv1EncNoColourDescription =>
                "The GStreamer libaom element writes no colour information, so every viewer would expand the picture "
                + "as limited range whatever is picked. It works on ffmpeg.",

            TextCode.GapSvtav1NoLossless =>
                "SVT-AV1 has no lossless mode.",

            TextCode.GapSvtav1NoConstrainedVbr =>
                "SVT-AV1 refuses a ceiling outside quality mode, so no build can cap its bursts. "
                + "Average bitrate is the same encode under a name that fits it.",

            TextCode.GapGstSvtav1EncNoCbr =>
                "The GStreamer SVT-AV1 element stalls in the mode constant bitrate needs. It works on ffmpeg.",

            TextCode.GapRav1ENoLossless =>
                "rav1e has no lossless mode.",

            TextCode.GapRav1ENoConstrainedVbr =>
                "rav1e takes one bitrate target and nothing above it, so no build can cap its bursts. "
                + "Average bitrate is the same encode under a name that fits it.",

            TextCode.GapAmfAv1LimitedRangeOnly =>
                "AMD's AV1 encoder marks everything as limited range whatever it is given, so a full-range stream "
                + "would arrive stretched at every viewer.",

            TextCode.GapVulkanAv1LimitedRangeOnly =>
                "The Vulkan AV1 encoder marks everything as limited range whatever it is given, so a full-range "
                + "stream would arrive stretched at every viewer.",

            TextCode.GapVaapiNoLossless =>
                "Intel and AMD's fixed-function encoders quantize every frame. No profile they implement is lossless.",

            TextCode.GapGstVaNoColourDescription =>
                "The GStreamer VAAPI elements write no colour information, so every viewer would expand the picture "
                + "as limited range whatever is picked. It works on ffmpeg.",

            TextCode.GapQsvNoLossless =>
                "Quick Sync quantizes every frame. Intel's runtime exposes no lossless path.",

            TextCode.GapGstAmfcodecWindowsOnly =>
                "The GStreamer AMD plugin is built for Windows only. AMD's encoders are reachable through ffmpeg.",

            TextCode.GapAmfNoLossless =>
                "AMD's fixed-function encoders quantize every frame. No profile they implement is lossless.",

            TextCode.GapGstVulkanNoCaptureMemory =>
                "The GStreamer Vulkan encoder reads frames no capture method on that engine produces. "
                + "Vulkan encoding is reachable through ffmpeg.",

            TextCode.GapVulkanNoLossless =>
                "Vulkan's lossless setting is a hint rather than a mode, and its encoders quantize under it anyway.",

            TextCode.GapVp8HasNoColourRangeField =>
                "VP8 has nowhere in its stream to record the colour range, so every viewer would expand the picture "
                + "as limited range. The other formats carry it and reach both.",

            TextCode.GapGstSoftwareNoRateCeiling =>
                "No GStreamer CPU encoder takes a ceiling above the target, so capped variable bitrate needs ffmpeg. "
                + "Average bitrate is the same encode under a name that fits it.",

            TextCode.GapGstElementsNoPlanarRgb =>
                "The GStreamer H.265, VP9 and AV1 elements take no RGB input. Sending the desktop's own pixels "
                + "needs a capture method that runs ffmpeg.",

            TextCode.GapGstAv1EncNoTune =>
                "This AV1 encoder aims at nothing in particular on a GStreamer capture method. Picking what it "
                + "optimises for needs a capture method that runs ffmpeg.",

            TextCode.GapGstQsvNoScenario =>
                "The GStreamer Quick Sync elements take no scenario, so telling the encoder what the session is "
                + "for needs a capture method that runs ffmpeg.",

            TextCode.GapVideotoolboxNoLossless =>
                "The Mac encoder compresses every frame and has no exact mode. Encoding on the CPU reaches one.",

            TextCode.GapVideotoolboxAverageBitrateOnly =>
                "The Mac encoder aims at an average bitrate and takes no ceiling and no quality target. "
                + "Average bitrate is the mode it codes.",

            // Diagnostics.

            TextCode.PublishRefused =>
                "These settings cannot be published as they stand.",

            // The audience and the wire.

            TextCode.StreamIsPublic =>
                "No group key is set, so this stream goes out where anyone who knows the relay address can watch it. "
                + "It is still encrypted on the way there. Create a group and hand the key to the people who should "
                + "see it, or leave this as it is if the stream is meant to be open.",

            TextCode.EncryptionFollowsTheAddress =>
                "Whether the connection is encrypted follows the relay address above and is not a setting, so this "
                + "box is a reading rather than a switch. A relay on this machine or the local network is reached "
                + "directly; anything else is encrypted, with no way to turn that off.",

            TextCode.EncryptedRtspInterleavesRtp =>
                "An encrypted RTSP session carries the video inside its TLS connection. Sending it over UDP would put "
                + "the picture on the wire beside that connection unencrypted, so TCP is the only choice here.",

            TextCode.SrtPassphraseIsTheEncryption =>
                "SRT has no TLS of its own, so this passphrase is what encrypts it. Without one the stream crosses the "
                + "internet readable by anyone on the way. It is the same value the relay is configured with.",

            TextCode.NoUplinkStated =>
                "No upload speed is set, so nothing checks the stream against the connection. Measure it or type it "
                + "in, and a configuration this line cannot carry will say so here instead of at the viewers.",

            TextCode.UplinkBelowPrediction =>
                $"This is predicted to need about {Decimal(a.BitrateMbps)} Mbit/s, and the upload speed is "
                + $"{Number(a.UplinkMbps)} Mbit/s. The encoder does not slow down to fit: the stream queues, packets "
                + "are dropped, and viewers see it stall rather than soften.",

            TextCode.BurstAboveUplink =>
                $"These settings run between about {Decimal(a.LowMbps)} and {Decimal(a.HighMbps)} Mbit/s depending on "
                + $"what is on screen, and the top of that is above the {Number(a.UplinkMbps)} Mbit/s upload speed. "
                + $"{Burst(a.Mode)} A still desktop will go out fine and a moving one will not.",

            TextCode.FpsAboveRefresh =>
                $"This asks for {Number(a.Fps)} frames a second from a screen that produces "
                + $"{Number(a.RefreshHz)}. The extra frames are copies of the last one: they cost bandwidth and add "
                + "no smoothness.",

            TextCode.MonitorNotPriced =>
                "The selected screen is not connected, so there is no picture size to predict from.",

            TextCode.NoPictureToPrice =>
                "These settings do not add up to a picture that can be predicted from.",

            TextCode.CompressionRatio => a.BitrateMbps > 0
                ? $"This screen produces {Decimal(a.RawMbps)} Mbit/s uncompressed, and this is predicted to send "
                  + $"{Decimal(a.BitrateMbps)} Mbit/s of it, about {Number((long)Math.Round(a.RawMbps / a.BitrateMbps))}:1."
                : $"This screen produces {Decimal(a.RawMbps)} Mbit/s uncompressed.",

            // Presets.

            // The sentence names the preset although it prints under the row that already does, so it still says
            // what it is about wherever it is shown.
            // The publish leg is the one dimension the search does not move, so it is what is left to change.
            TextCode.PresetUnreachable =>
                $"Nothing this machine can run delivers {Words.Preset(a.Preset)} over "
                + $"{Words.Transport(a.Transport)}. Nothing was changed: a near miss under this "
                + "name would be settings nobody asked for.",

            // Notices.

            TextCode.SettingsStoreUnreadable => a.Path.Length > 0
                ? $"The saved settings could not be read, so these are the defaults. The old file was kept as {a.Path}."
                : "The saved settings could not be read, so these are the defaults.",

            TextCode.PresetStoreUnreadable => a.Path.Length > 0
                ? $"The saved presets could not be read, so none are listed. The old file was kept as {a.Path}."
                : "The saved presets could not be read, so none are listed.",

            // A backend newer than this build.
            // The code is printed so it can be searched for and reported.
            _ => text.Code.ToString(),
        };
    }

    /// <summary>Whether a statement renders as anything.</summary>
    public static bool Any(Text? text) => Of(text).Length > 0;

    /// <summary>
    /// Why a mode's bandwidth spreads, for the sentence saying the top of the spread is too high.
    /// Follows the mode alone, so it sits inside that sentence rather than becoming three of them.
    /// </summary>
    private static string Burst(string mode) => mode switch
    {
        "crf" => "Constant quality sets no bandwidth bound at all, so what goes out is whatever the picture costs.",
        "lossless" => "A lossless encode spends whatever the picture costs and approaches the raw rate on motion.",
        "vbr" => "Variable bitrate averages toward the target and bursts to the ceiling above it.",
        _ => "",
    };

    /// <summary>
    /// A statement and the ways out it carries: "fact: do X, or do Y."
    /// One with no way out ends after the fact rather than promising one.
    /// </summary>
    private static string Ways(string fact, params string[] ways)
    {
        var offered = ways.Where(way => way.Length > 0).ToList();
        return offered.Count == 0 ? fact + "." : $"{fact}: {Words.List(offered)}.";
    }

    /// <summary>
    /// Several statements one after another, dropping the ones with nothing to say.
    /// Each already ends in its own punctuation, so they join with a space.
    /// </summary>
    private static string Sentences(string opening, params string[] rest)
    {
        var parts = new List<string> { opening.EndsWith('.') ? opening : opening + "." };
        parts.AddRange(rest.Where(part => part.Length > 0));
        return string.Join(" ", parts);
    }

    /// <summary>Whole figure, grouped as the reader's locale groups one.</summary>
    private static string Number(long value) => value.ToString("N0", CultureInfo.CurrentCulture);

    /// <summary>
    /// Whole figure with no grouping, for the ones that are identifiers rather than quantities: a pixel dimension
    /// is written 2560 everywhere, and a separator in one reads as a different number.
    /// </summary>
    private static string Plain(long value) => value.ToString(CultureInfo.InvariantCulture);

    /// <summary>
    /// A rate, to one decimal place under ten and to none above it.
    /// A stream is never precise to a hundredth of a megabit, and printing one implies it is.
    /// </summary>
    private static string Decimal(double value) =>
        value < 10 ? value.ToString("0.#", CultureInfo.CurrentCulture) : value.ToString("0", CultureInfo.CurrentCulture);

    /// <summary>
    /// A sentence lowercased at its first letter, for quoting one inside another.
    /// Only the first character moves, so an acronym further in is left alone.
    /// </summary>
    private static string Lower(string sentence) =>
        sentence.Length == 0 || char.IsLower(sentence[0]) ? sentence : char.ToLowerInvariant(sentence[0]) + sentence[1..];

    /// <summary>
    /// Arguments of one statement, read by name and never by position.
    /// A statement carries the arguments its facts needed and leaves out the rest, so a reader keyed on position
    /// would silently shift.
    /// </summary>
    private readonly struct Args(Text text)
    {
        private readonly Text _text = text;

        public string Capture => Id(TextArgName.Capture);

        public string Engine => Id(TextArgName.Engine);

        public string OtherEngine => Id(TextArgName.OtherEngine);

        public string Transport => Id(TextArgName.Transport);

        /// <summary>Built-in preset, not the NVENC ladder step <see cref="Effort"/> carries.</summary>
        public string Preset => Id(TextArgName.Preset);

        public string Codec => Id(TextArgName.Codec);

        public string Format => Id(TextArgName.Format);

        public string Encoder => Id(TextArgName.Encoder);

        public string Family => Id(TextArgName.Family);

        public string Chroma => Id(TextArgName.Chroma);

        public string ColorRange => Id(TextArgName.ColorRange);

        public string Mode => Id(TextArgName.Mode);

        public string Memory => Id(TextArgName.Memory);

        public string Audio => Id(TextArgName.Audio);

        public string Device => Id(TextArgName.Device);

        public string AudioCodec => Id(TextArgName.AudioCodec);

        public string Effort => Id(TextArgName.Effort);

        public string Tune => Id(TextArgName.Tune);

        public string Decoder => Id(TextArgName.Decoder);

        public string Os => Id(TextArgName.Os);

        public string Display => Id(TextArgName.Display);

        /// <summary>Video driver an encode runs through, e.g. "radeonsi".</summary>
        public string GpuDriver => Id(TextArgName.GpuDriver);

        /// <summary>Adapter that driver drives, e.g. "AMD Radeon 780M Graphics".</summary>
        public string GpuModel => Id(TextArgName.GpuModel);

        public string Path => Id(TextArgName.Path);

        public string Value => Id(TextArgName.Value);

        public string Element => Id(TextArgName.Element);

        public IReadOnlyList<string> Families => Ids(TextArgName.Families);

        public IReadOnlyList<string> Formats => Ids(TextArgName.Formats);

        public IReadOnlyList<string> Transports => Ids(TextArgName.Transports);

        public IReadOnlyList<string> AudioCodecs => Ids(TextArgName.AudioCodecs);

        public IReadOnlyList<string> DecodeFamilies => Ids(TextArgName.DecodeFamilies);

        public long Width => Num(TextArgName.Width);

        public long Height => Num(TextArgName.Height);

        public long Fps => Num(TextArgName.Fps);

        public long RefreshHz => Num(TextArgName.RefreshHz);

        public long MaxrateMbps => Num(TextArgName.MaxrateMbps);

        /// <summary>Top of the selected codec's quantizer scale on the engine behind the capture.</summary>
        public long CqMax => Num(TextArgName.CqMax);

        public long BitrateLimitMbps => Num(TextArgName.BitrateLimitMbps);
        public long GopLimitFrames => Num(TextArgName.GopLimitFrames);

        public long UplinkMbps => Num(TextArgName.UplinkMbps);

        public long RateHz => Num(TextArgName.RateHz);

        public long BitrateKbps => Num(TextArgName.BitrateKbps);

        /// <summary>
        /// Whole where it is a target the user set, fractional where it is a prediction.
        /// Both are read here, so a sentence quoting it need not know which it got.
        /// </summary>
        public double BitrateMbps => Dec(TextArgName.BitrateMbps);

        public double RawMbps => Dec(TextArgName.RawMbps);

        public double LowMbps => Dec(TextArgName.LowMbps);

        public double HighMbps => Dec(TextArgName.HighMbps);

        public Text? Cause => Nested(TextArgName.Cause);

        public Text? Cost => Nested(TextArgName.Cost);

        public Text? Reach => Nested(TextArgName.Reach);

        private string Id(TextArgName name) => Find(name)?.Id ?? "";

        private IReadOnlyList<string> Ids(TextArgName name) => Find(name)?.Ids?.Ids ?? (IReadOnlyList<string>)[];

        private long Num(TextArgName name)
        {
            var arg = Find(name);
            return arg?.ValueCase switch
            {
                TextArg.ValueOneofCase.Number => arg.Number,
                TextArg.ValueOneofCase.Decimal => (long)Math.Round(arg.Decimal),
                _ => 0,
            };
        }

        private double Dec(TextArgName name)
        {
            var arg = Find(name);
            return arg?.ValueCase switch
            {
                TextArg.ValueOneofCase.Decimal => arg.Decimal,
                TextArg.ValueOneofCase.Number => arg.Number,
                _ => 0,
            };
        }

        private Text? Nested(TextArgName name) => Find(name)?.Text;

        private TextArg? Find(TextArgName name)
        {
            foreach (var arg in _text.Args)
            {
                if (arg.Name == name)
                {
                    return arg;
                }
            }

            return null;
        }
    }
}
