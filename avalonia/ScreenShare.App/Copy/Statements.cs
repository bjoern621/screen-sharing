using System.Globalization;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Copy;

/// <summary>
/// Turns one statement from the backend into a sentence.
///
/// The backend sends no prose.
/// It sends a code naming the fact and the identifiers the fact is about,
/// capture <c>portal</c>, codec <c>libx264</c>, engine <c>gstreamer</c>,
/// and this is where that becomes something to read (api/proto/screenshare/v1/text.proto).
///
/// Every sentence names the limit, which side has it, and the way out,
/// so a grayed control leaves the reader something to do.
/// Where the backend hands over an alternative the sentence uses it,
/// and where it hands over none the sentence stops rather than trailing off.
/// Identifiers reach the screen through <see cref="Words"/>,
/// so a reader is told about an NVIDIA GPU rather than about <c>nvenc</c>,
/// and the identifier comes back only where the reader meets it again in a log or a command line.
///
/// An unknown code renders as the code: a shell older than its backend meets one,
/// and a blank where a reason belongs reads as a control grayed for nothing.
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
                + "On Wayland it sees only X11 windows. Use the screen picker instead.",

            // Prints as a fragment on the entry's own row, where a sentence would crowd out the name.
            // What the privilege is for is the entry's paragraph (Descriptions.Capture).
            TextCode.CaptureNeedsGrant => "needs elevated privileges",

            TextCode.CaptureTakesNoMonitor => a.Capture switch
            {
                "kmsgrab" => "This capture reads everything the GPU is scanning out, not one screen.",
                "gdigrab" => "This capture reads the whole desktop as one picture: every screen side by side.",
                "portal" => "The desktop's own picker chooses what is shared, so there is nothing to pick here.",
                _ => "This capture method chooses what it captures.",
            },

            // Each branch names what the session does,
            // so a missing picture reads as how the machine works.
            TextCode.NoMonitorPreview => a.Display == "wayland"
                ? "Wayland hands out screens only through the desktop's own picker, which asks "
                  + "every time. The picker shows what is shared when the stream starts."
                : "This system chooses the screen itself, so a preview of a single screen "
                  + "cannot be shown.",

            TextCode.MonitorNotEnumerated =>
                "Not connected. It stays selected, so what the stream would capture stays visible. "
                + "Pick a screen that is plugged in.",

            TextCode.ScaledFromSource =>
                $"from {Plain(a.Width)} × {Plain(a.Height)}",

            // Publish engines and the encoder probe.

            TextCode.EngineToolingMissing =>
                $"{Words.Engine(a.Engine)} is not installed, so its encoders cannot run. "
                + "Install it, or pick a capture method that uses the other engine.",

            TextCode.EngineHasNoPublishSink =>
                $"{Words.Capture(a.Capture)} runs on {Words.Engine(a.Engine)}, which cannot send over "
                + $"{Words.Transport(a.Transport)}. Change the capture method to reach it.",

            TextCode.PublishSinkElementMissing =>
                $"Sending over {Words.Transport(a.Transport)} needs {a.Element}, which this "
                + $"{Words.Engine(a.Engine)} install does not include. Install the plugin that provides it, "
                + "or pick another protocol.",

            TextCode.EngineNotProbed =>
                $"No encoder on {Words.Engine(a.Engine)} has been tested on this computer, because "
                + $"{Lower(Of(a.Cause))} Every choice stays offered, so one this computer cannot run "
                + "stops at the start instead of here.",

            TextCode.ProbeNoDevice =>
                $"No {Words.Family(a.Family)} encoder was found on this computer. Either the hardware is missing, "
                + "or its driver does not offer encoding.",

            TextCode.ProbeNoBuild =>
                $"This install has no {a.Codec} encoder. The software is missing, not the hardware. "
                + "A different build or package provides it.",

            TextCode.ProbeFailed =>
                $"{Words.Engine(a.Engine)} could not run {a.Codec} on this computer.",

            TextCode.CodecNotImplemented =>
                "This format cannot be encoded here.",

            TextCode.NoEncoderForFormat =>
                $"No encoder on this computer produces {Words.Format(a.Format)}. Pick another format.",

            TextCode.EncoderCodesNoFormat => a.Formats.Count > 0
                ? $"{Words.Encoder(a.Encoder)} encodes {Words.List(a.Formats.Select(Words.Format), "and")}, "
                  + $"not {Words.Format(a.Format)}."
                : $"{Words.Encoder(a.Encoder)} does not encode {Words.Format(a.Format)}.",

            // Carriage.

            TextCode.TransportCarriesNoFormat => Ways(
                $"{Words.Transport(a.Transport)} cannot carry {Words.Format(a.Format)} on {Words.Engine(a.Engine)}",
                a.Transports.Count > 0
                    ? $"send it over {Words.List(a.Transports.Select(Words.Transport))}"
                    : "",
                a.OtherEngine.Length > 0
                    ? $"keep {Words.Transport(a.Transport)} and pick a capture method that uses {Words.Engine(a.OtherEngine)}"
                    : ""),

            TextCode.LegCarriesNoAudioCodec => a.AudioCodecs.Count > 0
                ? $"{Words.Transport(a.Transport)} carries no {Words.AudioCodec(a.AudioCodec)} track on "
                  + $"{Words.Engine(a.Engine)}. It carries {Words.List(a.AudioCodecs.Select(Words.AudioCodec))}."
                : $"{Words.Transport(a.Transport)} carries no audio on {Words.Engine(a.Engine)}. "
                  + "Send over another protocol, or turn audio off.",

            TextCode.RenderChainElementMissing =>
                $"This computer has no {a.Element}, which {Words.RenderChain(a.Value)} needs. "
                + "Pick another route to convert the frames.",

            TextCode.EngineHasNoAudioEncoder =>
                $"{Words.Engine(a.Engine)} has no {Words.AudioCodec(a.AudioCodec)} encoder. "
                + "Change the capture method to reach the other engine.",

            TextCode.NoViewerReceivesOver =>
                $"No built-in player opens {Words.Transport(a.Transport)}. "
                + $"Watch over {Words.List(a.Transports.Select(Words.Transport))}.",

            TextCode.RelayServesNoFormatOver => a.Transports.Count > 0
                ? $"The relay does not serve {Words.Format(a.Format)} over {Words.Transport(a.Transport)}, so a "
                  + $"player would connect and receive nothing. Watch over {Words.List(a.Transports.Select(Words.Transport))}."
                : $"The relay does not serve {Words.Format(a.Format)} over {Words.Transport(a.Transport)}, so a "
                  + "player would connect and receive nothing.",

            // Pixel format, color and decoding.

            TextCode.CodecCodesNoRgb =>
                "This encoder cannot take the desktop's RGB pixels directly. Only some H.265 and VP9 encoders "
                + "can. Pick 4:4:4 for the closest result.",

            TextCode.CodecCannotEncodeChroma =>
                $"This encoder cannot produce {Words.Chroma(a.Chroma)}. Most hardware encoders produce only "
                + "4:2:0. The CPU encoders cover the rest.",

            TextCode.RgbIsFullRange =>
                "RGB uses every code value by definition, so there is nothing to choose here.",

            TextCode.DecodesInHardware =>
                $"Viewers decode this on the GPU on {Words.List(a.DecodeFamilies.Select(Words.DecodeFamily), "and")}.",

            TextCode.DecodesOnCpu =>
                $"No GPU decodes {Words.Format(a.Format)} at {Words.Chroma(a.Chroma)}, so every viewer decodes it "
                + "on the CPU"
                + (a.Decoder.Length > 0 ? $" ({a.Decoder})" : "")
                + ". It plays everywhere, at the cost of viewer CPU.",

            TextCode.DecodesInHardwarePartly =>
                $"{Words.Chroma(a.Chroma)} {Words.Format(a.Format)} decodes on the GPU only on "
                + $"{Words.List(a.DecodeFamilies.Select(Words.DecodeFamily), "and")}. "
                + "Every other viewer decodes it on the CPU.",

            // Frame memory and the GPU path.

            TextCode.PairHasNoDeviceMemory =>
                $"{Words.Capture(a.Capture)} and this encoder share no memory on {Words.Engine(a.Engine)}, "
                + "so the frames go through main memory whatever this is set to.",

            TextCode.PairConvertsOnDevice =>
                $"{Words.Capture(a.Capture)} and this encoder convert on the GPU with the color chosen here. "
                + $"\"{Words.Memory(a.Memory)}\" keeps both.",

            TextCode.PairTradesColour => Sentences(
                $"{Words.Capture(a.Capture)} and this encoder can share memory on {Words.Engine(a.Engine)}, but "
                + "nothing between them converts the color",
                Of(a.Cost),
                Of(a.Reach)),

            TextCode.ExactColourReach =>
                $"{Words.Capture(a.Capture)} on {Words.Engine(a.Engine)} converts on the GPU, "
                + "so the frames stay there in the color chosen here.",

            TextCode.DevicePathHasNoScaler =>
                "On this path the frames never leave the GPU, and nothing on it can resize them. "
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
                "The NVIDIA encoder reads the captured texture on its own device, with no conversion in between.",

            TextCode.CostEncoderSignalsItsOwnColour =>
                $"The encoder converts the picture itself and sends {Words.Chroma(a.Chroma)} at "
                + $"{Words.ColorRange(a.ColorRange)} whatever is picked here.",

            // Rate control.

            TextCode.CqOnlyInConstantQuality =>
                "Only constant quality has a quality target. Switch to it to set one.",

            TextCode.BitrateNotInMode =>
                $"{Words.Mode(a.Mode)} does not target a bitrate. The rate follows the picture instead.",

            // The pointer.

            TextCode.KmsgrabHasNoCursorPlane =>
                $"{Words.Capture(a.Capture)} reads the screen as it is scanned out. The display hardware "
                + "draws the pointer later, so it never appears in the capture.",

            TextCode.CaptureHasNoCursorMetadata =>
                $"{Words.Capture(a.Capture)} delivers the picture and nothing else. "
                + "The desktop portal and the X11 screen capture report where the pointer is.",

            TextCode.FormatCarriesNoCursorMetadata =>
                $"{Words.Format(a.Format)} carries the picture and nothing beside it, so viewers would see no pointer. "
                + "H.264 and HEVC carry the position to them.",

            TextCode.PortalServesNoCursorMode =>
                $"{Words.Capture(a.Capture)} passes on the pointer position the desktop reports. "
                + "This desktop reports none, so draw the pointer into the picture or leave it out.",

            TextCode.CqAboveCodecScale =>
                $"The quality scale of {a.Codec} ends at {a.CqMax} on {Words.Engine(a.Engine)}, so the slider "
                + "stops there. Each encoder has its own scale, and the same number means a different quality "
                + "on each.",

            TextCode.BitrateAboveCodecLimit =>
                $"{a.Codec} does not accept a target above {a.BitrateLimitMbps} Mbit/s on {Words.Engine(a.Engine)}. "
                + "It's the encoder's own limit, and an encode asking for more does not start.",

            TextCode.GopAboveCodecLimit =>
                $"{a.Codec} accepts a keyframe interval of at most {a.GopLimitFrames} frames on "
                + $"{Words.Engine(a.Engine)}. An encode asking for a longer one does not start.",

            TextCode.DriverDefectWithholdsOption =>
                $"{a.Codec} supports this, but the graphics driver here crashes on it. On {a.GpuModel} "
                + "such a stream stops partway and takes the picture down with it. "
                + "The other choices on this control work.",

            TextCode.MaxrateOnlyInConstrainedVbr =>
                "A ceiling applies only in the modes that let the rate move: capped variable bitrate and "
                + "constant quality. The other modes hold the target itself.",

            TextCode.VbvOnlyInBoundedModes =>
                "The buffer matters only while the encoder holds a bitrate.",

            TextCode.NoCeilingInConstantQuality =>
                $"In constant quality {a.Codec} takes no ceiling: the rate follows the picture. "
                + "Capped variable bitrate bounds the rate on this encoder.",

            TextCode.VbvNeedsACeiling =>
                "The buffer sets how long the stream may stay above the ceiling. Set a ceiling first.",

            TextCode.BframesOffInMode =>
                $"{Words.Mode(a.Mode)} turns these off. They would add delay and save nothing in this mode.",

            TextCode.BframesOnlyOnFamilies =>
                $"Only the {Words.List(a.Families.Select(Words.Family), "and")} encoders accept these.",

            TextCode.CodecTakesNoEffortLadder =>
                $"{a.Codec} has no effort setting. The encoder decides on its own how hard it works.",

            TextCode.EffortPinnedByMode =>
                $"{Words.Mode(a.Mode)} pins this to {Words.Effort(a.Effort)} for its low-delay tuning.",

            TextCode.CodecTakesNoTuneLadder =>
                $"{a.Codec} has no tune setting. It encodes the picture the same way whatever it contains.",

            TextCode.TunePinnedByMode =>
                $"{Words.Mode(a.Mode)} pins this to {Words.Tune(a.Tune)}.",

            TextCode.AudioCodecNeedsSource =>
                "No audio is being sent, so there is nothing to compress. Add a source to pick a format.",

            TextCode.EngineTagsStandardRange =>
                $"{Words.Engine(a.Engine)} tags every stream as standard range and cannot read what this screen "
                + $"produces. Ten bits adds precision here, not HDR. {Words.Engine(a.OtherEngine)} carries HDR.",

            TextCode.AudioEntryNeedsSource =>
                "Pick where this row records from first.",

            TextCode.AudioSourceHasOneDevice =>
                $"{Words.AudioSource(a.Audio)} has one device on this computer, so there is nothing to choose between.",

            TextCode.AudioDeviceNotEnumerated =>
                "not available right now",

            TextCode.AudioSourceUnservedByEngine =>
                $"{Words.Engine(a.Engine)} cannot record this source. "
                + $"Pick a capture method that uses {Words.Engine(a.OtherEngine)}.",

            TextCode.AudioSourceUnserved => a.Os switch
            {
                "windows" => "Windows exposes one program's sound only by its process id, which cannot be "
                    + "looked up here. Record everything this computer plays instead.",
                "darwin" => "macOS offers no system-audio input device, so desktop sound cannot be recorded. "
                    + "A loopback driver is the usual workaround.",
                _ => $"{Words.OperatingSystem(a.Os)} offers no device either encoder can record from.",
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
                "The GStreamer rav1e element has no keyframe-interval setting. Its own default applies.",

            TextCode.GstNvencNoRateBuffer =>
                "The GStreamer NVIDIA elements have no rate-buffer setting.",

            TextCode.GstQsvNoRateBuffer =>
                "The GStreamer Quick Sync elements have no rate-buffer setting. It works on ffmpeg.",

            TextCode.QualityCeilingRequired =>
                $"{a.Codec} has no unbounded quality mode. It always encodes toward a ceiling, "
                + "so this figure is required.",

            TextCode.FixedFunctionAbrDerivesCeiling =>
                "Hardware encoders always encode against a ceiling, so the ceiling is set to twice this "
                + "target. The target holds the average.",

            TextCode.AmfCodesNoBframes =>
                "AMD's encoders run with look-ahead frames off, so a live stream avoids their delay.",

            TextCode.FfmpegVaapiQualityIsTheDriversScale =>
                "The graphics driver decides how hard this encoder works, on a scale of its own. "
                + "A capture method that uses GStreamer offers the setting.",

            TextCode.VaapiCeilingBound =>
                $"On this encoder the ceiling cannot exceed {Number(a.MaxrateMbps)} Mbit/s against a "
                + $"{Decimal(a.BitrateMbps)} Mbit/s target. The target is set as a percentage of the ceiling, "
                + "and half is the minimum.",

            // Codec capability gaps.

            TextCode.GapNvencAv1NoLosslessTune =>
                "NVIDIA's AV1 encoder has no lossless mode. Its H.264 and H.265 encoders have one.",

            TextCode.GapGstVp9EncNoLossless =>
                "The GStreamer VP9 element has no lossless setting. It works on ffmpeg.",

            TextCode.GapVp8HasNoLossless =>
                "VP8 has no lossless mode. VP9 has one.",

            TextCode.GapGstAv1EncEightBitOnly =>
                "The GStreamer libaom element takes 8-bit input only. 10-bit works on ffmpeg.",

            TextCode.GapLibaomNoLosslessSwitch =>
                "Neither build exposes libaom's lossless switch.",

            TextCode.GapGstAv1EncNoColourDescription =>
                "The GStreamer libaom element writes no color information, so every viewer would expand the "
                + "picture as limited range whatever is picked. It works on ffmpeg.",

            TextCode.GapSvtav1NoLossless =>
                "SVT-AV1 has no lossless mode.",

            TextCode.GapSvtav1NoConstrainedVbr =>
                "SVT-AV1 accepts no ceiling outside quality mode, so its bursts cannot be capped. "
                + "Average bitrate produces the same encode.",

            TextCode.GapGstSvtav1EncNoCbr =>
                "The GStreamer SVT-AV1 element stalls in the mode constant bitrate needs. It works on ffmpeg.",

            TextCode.GapRav1ENoLossless =>
                "rav1e has no lossless mode.",

            TextCode.GapRav1ENoConstrainedVbr =>
                "rav1e takes one bitrate target and no ceiling, so its bursts cannot be capped. "
                + "Average bitrate produces the same encode.",

            TextCode.GapAmfAv1LimitedRangeOnly =>
                "AMD's AV1 encoder marks everything as limited range whatever it is given, so a full-range "
                + "stream would arrive stretched at every viewer.",

            TextCode.GapVulkanAv1LimitedRangeOnly =>
                "The Vulkan AV1 encoder marks everything as limited range whatever it is given, so a "
                + "full-range stream would arrive stretched at every viewer.",

            TextCode.GapVaapiNoLossless =>
                "Intel and AMD's fixed-function encoders quantize every frame. No profile they implement "
                + "is lossless.",

            TextCode.GapGstVaNoColourDescription =>
                "The GStreamer VAAPI elements write no color information, so every viewer would expand the "
                + "picture as limited range whatever is picked. It works on ffmpeg.",

            TextCode.GapQsvNoLossless =>
                "Quick Sync quantizes every frame. Intel's runtime exposes no lossless path.",

            TextCode.GapGstAmfcodecWindowsOnly =>
                "The GStreamer AMD plugin is built for Windows only. AMD's encoders are available through ffmpeg.",

            TextCode.GapAmfNoLossless =>
                "AMD's fixed-function encoders quantize every frame. No profile they implement is lossless.",

            TextCode.GapGstVulkanNoCaptureMemory =>
                "The GStreamer Vulkan encoder reads frames no capture method on that engine produces. "
                + "Vulkan encoding is available through ffmpeg.",

            TextCode.GapVulkanNoLossless =>
                "Vulkan's lossless setting is a hint rather than a mode, and its encoders quantize under "
                + "it anyway.",

            TextCode.GapVp8HasNoColourRangeField =>
                "VP8 has nowhere in its stream to record the color range, so every viewer would expand the "
                + "picture as limited range. The other formats carry the range.",

            TextCode.GapGstSoftwareNoRateCeiling =>
                "No GStreamer CPU encoder takes a ceiling above the target, so capped variable bitrate needs "
                + "ffmpeg. Average bitrate produces the same encode.",

            TextCode.GapGstElementsNoPlanarRgb =>
                "The GStreamer H.265, VP9 and AV1 elements take no RGB input. Sending the desktop's own "
                + "pixels needs a capture method that uses ffmpeg.",

            TextCode.GapGstAv1EncNoTune =>
                "This AV1 encoder has no tune setting on a GStreamer capture method. Picking one needs a "
                + "capture method that uses ffmpeg.",

            TextCode.GapGstQsvNoScenario =>
                "The GStreamer Quick Sync elements take no scenario setting. Setting one needs a capture "
                + "method that uses ffmpeg.",

            TextCode.GapVideotoolboxNoLossless =>
                "The Mac hardware encoder compresses every frame and has no lossless mode. "
                + "The CPU encoders have one.",

            TextCode.GapVideotoolboxAverageBitrateOnly =>
                "The Mac hardware encoder targets an average bitrate only, with no ceiling and no quality target.",

            // Diagnostics.

            TextCode.PublishRefused =>
                "These settings cannot be published as they stand.",

            // Names the two settings membership follows from:
            // everything else about a close is the child's own words, printed beside this.
            TextCode.GroupMembershipLapsed =>
                "This computer is not a member of the group, so the relay closes its streams there. "
                + "Check the group key and the name under Relay.",

            // Names the group: the member holding the name is an id on the wire,
            // and an id beside a name a reader chose is a string they cannot act on.
            TextCode.GroupNameTaken =>
                "Another member of this group holds that name. Pick a different one under Relay.",

            TextCode.GroupNameMissing =>
                "A group needs a name for this computer. Set one under Relay.",

            // The nested statement is the relay's own, carrying what the reader can do about it.
            // Quoted through the same recursion a cost or a reach is (Sentences).
            TextCode.GroupServiceRefused => Sentences(
                "The relay refused this computer's membership in the group",
                Of(a.Cause)),

            TextCode.StreamLeftTheRelay =>
                "The stream stopped arriving at the relay, so there is nothing to receive on this path.",

            TextCode.TransportFallingBack =>
                $"{Words.Transport(a.Transport)} is not getting through. "
                + $"The stream tries {Words.Transport(a.NextTransport)} next.",

            // The audience and the wire.

            TextCode.GroupRequired =>
                "Sharing needs a group, and this computer is in none. Set a group key to join one, "
                + "or create a group and share its key.",

            TextCode.GroupFollowsDiscord =>
                "The group follows the voice channel while Follow Discord is on. "
                + "Turn it off to set a key and a name by hand.",

            TextCode.DiscordNotLinked =>
                "Follow Discord is on, but this computer is not linked to a Discord account. "
                + "Press Link Discord under Relay.",

            TextCode.DiscordNoVoiceChannel =>
                "Not in a voice channel. Join one in Discord to get a group.",

            TextCode.EncryptionFollowsTheAddress =>
                "Encryption follows the relay address above. This box shows the result rather than setting it. "
                + "A relay on this computer or the local network is reached directly. Anything further away "
                + "is always encrypted.",

            TextCode.EncryptedRtspInterleavesRtp =>
                "An encrypted RTSP session carries the video inside its TLS connection. UDP would put the "
                + "picture on the wire unencrypted, so TCP is the only choice here.",

            TextCode.NoUplinkStated =>
                "No upload speed is set, so the stream is not checked against the connection. Measure or "
                + "enter it. A configuration the connection cannot carry then shows up here instead of at "
                + "the viewers.",

            TextCode.UplinkBelowPrediction =>
                $"This stream is predicted to need about {Decimal(a.BitrateMbps)} Mbit/s, but the upload "
                + $"speed is {Number(a.UplinkMbps)} Mbit/s. The encoder does not slow down to fit. "
                + "Packets get dropped and viewers see the stream stall.",

            TextCode.BurstAboveUplink =>
                $"These settings need between about {Decimal(a.LowMbps)} and {Decimal(a.HighMbps)} Mbit/s, "
                + "depending on what is on screen. The peak is above the "
                + $"{Number(a.UplinkMbps)} Mbit/s upload speed. "
                + $"{Burst(a.Mode)} A still desktop goes out fine, a moving one does not.",

            TextCode.FpsAboveRefresh =>
                $"This asks for {Number(a.Fps)} frames a second from a screen that produces "
                + $"{Number(a.RefreshHz)}. The extra frames are copies. They cost bandwidth and add "
                + "no smoothness.",

            TextCode.MonitorNotPriced =>
                "The selected screen is not connected, so there is no picture size to predict from.",

            TextCode.NoPictureToPrice =>
                "No prediction is possible from these settings.",

            TextCode.CeilingHoldsQuality =>
                $"This quality target is predicted to need about {Decimal(a.BitrateMbps)} Mbit/s, above the "
                + $"{Number(a.MaxrateMbps)} Mbit/s ceiling. The encoder stops at the ceiling and softens the "
                + "picture instead, so a higher target changes nothing. Raise the ceiling to let it spend more.",

            // Presets.

            // The sentence names the preset although it prints under the row that already does, so it still says
            // what it is about wherever it is shown.
            TextCode.PresetUnreachable =>
                $"This computer cannot deliver {Words.Preset(a.Preset)} over "
                + $"{Words.Transport(a.Transport)}. The settings were left unchanged.",

            // Notices.

            TextCode.SettingsStoreUnreadable => a.Path.Length > 0
                ? $"The saved settings could not be read, so these are the defaults. The unreadable file was kept as {a.Path}."
                : "The saved settings could not be read, so these are the defaults.",

            TextCode.PresetStoreUnreadable => a.Path.Length > 0
                ? $"The saved presets could not be read, so none are listed. The unreadable file was kept as {a.Path}."
                : "The saved presets could not be read, so none are listed.",

            // Legs of a relay check nothing here uses.
            // Each says what is so,
            // and a reader who never heard of these listeners gets a fact about their relay.

            TextCode.RelayLegNoRelay =>
                "No relay is set, so there is no address to check.",

            TextCode.RelayLegDiscordOff =>
                "Discord mode is off, so nothing here uses this manager.",

            // Updates.
            // The first three are why nothing is checked or installed here, and each names what does it instead.

            TextCode.UpdateCheckOff =>
                "Update checks are off in this environment (MIRRORME_UPDATE_CHECK=0).",

            TextCode.UpdateBuildUnstamped =>
                "This build carries no version number, so there is nothing to compare a release against.",

            TextCode.UpdateChannelManaged =>
                $"{Words.Channel(a.Channel)} installed this copy, so updates come from there.",

            TextCode.UpdateServiceUnreadable =>
                "The release page could not be reached. Check the network and try again.",

            TextCode.UpdateNoDownload =>
                $"Release {a.Version} carries no download for this install. Get it from the release page.",

            TextCode.UpdateDownloadUnverifiable =>
                $"Release {a.Version} publishes no checksum for its download, so it was not installed. "
                + "Get it from the release page.",

            TextCode.UpdateDownloadFailed =>
                "The download stopped before it finished. Try again.",

            TextCode.UpdateDownloadCorrupt =>
                "What arrived does not match the published checksum, so it was deleted. Try again.",

            TextCode.UpdateInstallFailed =>
                "The staged release could not be started. Get it from the release page instead.",

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
        "crf" => "Constant quality sets no upper bound, so the rate follows the picture.",
        "lossless" => "A lossless encode approaches the raw rate on motion.",
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
    /// Whole figure with no grouping, for the ones that are identifiers rather than quantities:
    /// a pixel dimension is written 2560 everywhere, and a separator in one reads as a different number.
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
    /// Arguments of one statement, read by name.
    /// A statement carries the arguments its facts needed and leaves out the rest,
    /// so a reader keyed on position would silently shift.
    /// </summary>
    private readonly struct Args(Text text)
    {
        private readonly Text _text = text;

        public string Capture => Id(TextArgName.Capture);

        public string Engine => Id(TextArgName.Engine);

        public string OtherEngine => Id(TextArgName.OtherEngine);

        public string Transport => Id(TextArgName.Transport);

        /// <summary>The leg a pending relaunch runs, beside <see cref="Transport"/> carrying the one given up.</summary>
        public string NextTransport => Id(TextArgName.NextTransport);

        /// <summary>Built-in preset. <see cref="Effort"/> carries the NVENC ladder step also called one.</summary>
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

        /// <summary>Where this copy of the app came from, e.g. "nix", "pacman".</summary>
        public string Channel => Id(TextArgName.Channel);

        /// <summary>A release, as its tag spells it: "v0.5.1".</summary>
        public string Version => Id(TextArgName.Version);

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
        /// Whole where it is a typed target, fractional where it is a prediction.
        /// Both are read here, so a sentence quoting it need not know which it got.
        /// </summary>
        public double BitrateMbps => Dec(TextArgName.BitrateMbps);

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
