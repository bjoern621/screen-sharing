import {
    IconCheck, IconX, IconPlugConnected, IconDeviceDesktop,
    IconVideo, IconGauge, IconNetwork,
} from "@tabler/icons-react";
import { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { FieldSet, FieldLegend } from "@/components/ui/field";
import { BrowserVerdict, Deps, Monitor, Option, Stream } from "../../types/stream";
import {
    CAPTURES, CHROMAS, CODECS, ENC_PRESETS, MODES, RANGES,
    TRANSPORT_META, monitorOptions,
} from "../../util/options";
import { estimateBitrate, formatEstimate } from "../../util/estimate";
import Tip from "../Tip/Tip";
import ErrorLog from "../ErrorLog/ErrorLog";
import SelectField from "../fields/SelectField";
import NumberField from "../fields/NumberField";
import TextField from "../fields/TextField";
import UplinkField from "../fields/UplinkField";

/** Auto-fitting field grid shared by every settings section. */
const GRID =
    "grid gap-x-5 gap-y-3 [grid-template-columns:repeat(auto-fit,minmax(230px,1fr))]";

/** One titled settings section: an icon+label legend over a field grid. */
function Section({
    icon,
    title,
    children,
}: {
    icon: ReactNode;
    title: string;
    children: ReactNode;
}) {
    return (
        <FieldSet>
            <FieldLegend className="flex items-center gap-2">
                {icon}
                {title}
            </FieldLegend>
            <div className={GRID}>{children}</div>
        </FieldSet>
    );
}

interface UplinkState {
    measuring: boolean;
    error: string;
    remeasure: () => void;
}

interface StreamSettingsCardProps {
    s: Stream;
    deps: Deps;
    transports: string[];
    monitors: Monitor[];
    browser: BrowserVerdict;
    cmd: string;
    publishing: boolean;
    pubError: string;
    pubLogPath: string;
    uplink: UplinkState;
    onUpdate: (patch: Partial<Stream>) => void;
    onTogglePublish: () => void;
    onSave: () => void;
    onOpenLog: (path: string) => void;
    onOpenLogsFolder: () => void;
}

/** The full stream-settings form: dependency-aware fields, publish controls,
 * browser-viewability verdict and the live ffmpeg command preview. */
export default function StreamSettingsCard({
    s,
    deps,
    transports,
    monitors,
    browser,
    cmd,
    publishing,
    pubError,
    pubLogPath,
    uplink,
    onUpdate,
    onTogglePublish,
    onSave,
    onOpenLog,
    onOpenLogsFolder,
}: StreamSettingsCardProps) {
    const transportOptions: Option[] = transports.map(
        t => TRANSPORT_META[t] ?? { value: t, label: t, tip: t }
    );

    const selectedMonitor = monitors.find(m => m.index === s.monitor);
    const estimate = estimateBitrate(
        s,
        selectedMonitor?.width ?? 0,
        selectedMonitor?.height ?? 0
    );
    const resLabel =
        selectedMonitor && selectedMonitor.width && selectedMonitor.height
            ? `${selectedMonitor.width}×${selectedMonitor.height} @ ${s.fps} fps`
            : `@ ${s.fps} fps`;

    return (
        <Card>
            <CardHeader>
                <CardTitle className="text-base">Stream settings</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                <Section
                    icon={<IconPlugConnected size={16} className="text-muted-foreground" />}
                    title="Connection"
                >
                    <TextField
                        label="Stream name"
                        labelTip="Path name on the relay. Friends see and open this name."
                        value={s.name}
                        onChange={v => onUpdate({ name: v })}
                    />
                    <TextField
                        label="Relay host"
                        labelTip="Host running the MediaMTX relay. All publishers push to it, all viewers pull from it."
                        value={s.relayHost}
                        onChange={v => onUpdate({ relayHost: v })}
                    />
                    <NumberField
                        label="Relay port (SRT, UDP)"
                        labelTip="UDP port of the relay's SRT listener (default 8890)."
                        value={s.relayPort}
                        onChange={v => onUpdate({ relayPort: v })}
                    />
                    <NumberField
                        label="Relay API port (HTTP)"
                        labelTip="TCP port of the relay's HTTP API (default 9997), used for the Live-now list."
                        value={s.apiPort}
                        onChange={v => onUpdate({ apiPort: v })}
                    />
                </Section>

                <Section
                    icon={<IconDeviceDesktop size={16} className="text-muted-foreground" />}
                    title="Source"
                >
                    <SelectField
                        label="Capture API"
                        labelTip="Screen capture API feeding the encoder."
                        value={s.capture}
                        options={CAPTURES}
                        optionDisabled={deps.optionDisabled.capture}
                        onChange={v => onUpdate({ capture: v })}
                    />
                    <SelectField
                        label="Monitor"
                        labelTip="Which monitor to capture (one DXGI output per index)."
                        value={String(s.monitor)}
                        options={monitorOptions(monitors, s.monitor)}
                        disabledReason={deps.disabled.monitor}
                        onChange={v => onUpdate({ monitor: parseInt(v, 10) || 0 })}
                    />
                    <NumberField
                        label="Frame rate (fps)"
                        labelTip="Capture and encode frame rate. Higher = smoother motion, proportionally more encode load and bandwidth."
                        value={s.fps}
                        onChange={v => onUpdate({ fps: v })}
                    />
                </Section>

                <Section
                    icon={<IconVideo size={16} className="text-muted-foreground" />}
                    title="Encoder"
                >
                    <SelectField
                        label="Video codec"
                        labelTip="Video coding standard and implementation. NVENC variants run on the GPU's dedicated encoder ASIC; libx264 is software."
                        value={s.codec}
                        options={CODECS}
                        optionDisabled={deps.optionDisabled.codec}
                        onChange={v => onUpdate({ codec: v })}
                    />
                    <SelectField
                        label="Pixel format / chroma subsampling"
                        labelTip="Pixel format: the color model and chroma subsampling the encoder codes."
                        value={s.chroma}
                        options={CHROMAS}
                        optionDisabled={deps.optionDisabled.chroma}
                        onChange={v => onUpdate({ chroma: v })}
                    />
                    <SelectField
                        label="Color quantization range"
                        labelTip="Quantization range for Y′CbCr code values. Ignored for gbrp - RGB is inherently full range."
                        value={s.colorRange}
                        options={RANGES}
                        disabledReason={deps.disabled.colorRange}
                        onChange={v => onUpdate({ colorRange: v })}
                    />
                    <SelectField
                        label="Encoder preset (NVENC p1-p7)"
                        labelTip="NVENC preset ladder: p1 (fastest) to p7 (most efficient compression)."
                        value={s.encPreset}
                        options={ENC_PRESETS}
                        disabledReason={deps.disabled.encPreset}
                        onChange={v => onUpdate({ encPreset: v })}
                    />
                </Section>

                <Section
                    icon={<IconGauge size={16} className="text-muted-foreground" />}
                    title="Quality & rate control"
                >
                    <SelectField
                        label="Rate control mode"
                        labelTip="Rate-control strategy: how the encoder distributes bits over time."
                        value={s.mode}
                        options={MODES}
                        onChange={v => onUpdate({ mode: v })}
                    />
                    <NumberField
                        label="Quantizer target (CQ)"
                        labelTip="Constant quantizer for quality mode (rate-distortion tradeoff). Lower = better + more bits. 12 ≈ visually lossless, 19 ≈ excellent, 28 ≈ visibly compressed."
                        value={s.cq}
                        disabledReason={deps.disabled.cq}
                        onChange={v => onUpdate({ cq: v })}
                    />
                    <NumberField
                        label="Bitrate bound (Mbit/s)"
                        labelTip="Quality mode: burst ceiling only. Latency mode: constant target. Lossless: ignored."
                        value={s.bitrateM}
                        disabledReason={deps.disabled.bitrateM}
                        onChange={v => onUpdate({ bitrateM: v })}
                    />
                    <NumberField
                        label="GOP length (frames, 0 = 2×fps)"
                        labelTip="Group of Pictures: frames between keyframes. New viewers wait up to GOP/fps seconds to join; loss corrupts until the next keyframe. Long GOP = less bandwidth, slower joins."
                        value={s.gop}
                        onChange={v => onUpdate({ gop: v })}
                    />
                    <NumberField
                        label="B-frames"
                        labelTip="Bi-directionally predicted frames: reference past AND future. Save bitrate in lossy modes, add reorder latency; nothing in lossless."
                        value={s.bframes}
                        disabledReason={deps.disabled.bframes}
                        onChange={v => onUpdate({ bframes: v })}
                    />
                </Section>

                <Section
                    icon={<IconNetwork size={16} className="text-muted-foreground" />}
                    title="Network"
                >
                    <SelectField
                        label="Transport protocol"
                        labelTip="How the encoded stream is carried across the network from publisher to relay to viewer. Protocols differ in reliability, latency control and which players can receive them; pick one, then see each option's own tooltip."
                        value={s.transport}
                        options={transportOptions}
                        onChange={v => onUpdate({ transport: v })}
                    />
                    <NumberField
                        label="SRT publish latency (ms, hop 1)"
                        labelTip="SRT retransmit window for YOUR hop (publisher to relay). Total glass-to-glass ≈ hop 1 + hop 2 + encode/decode - the windows ADD UP."
                        value={s.srtPublishLatencyMs}
                        onChange={v => onUpdate({ srtPublishLatencyMs: v })}
                    />
                    <UplinkField
                        value={s.uplinkMbps}
                        measuring={uplink.measuring}
                        error={uplink.error}
                        onChange={v => onUpdate({ uplinkMbps: v })}
                        onRemeasure={uplink.remeasure}
                    />
                </Section>

                <div className="flex flex-wrap items-center gap-3">
                    <Button
                        onClick={onTogglePublish}
                        variant={publishing ? "destructive" : "default"}
                    >
                        {publishing && (
                            <span className="relative flex size-2">
                                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-current opacity-75" />
                                <span className="relative inline-flex size-2 rounded-full bg-current" />
                            </span>
                        )}
                        {publishing ? "Stop publishing" : "Start publishing"}
                    </Button>
                    <Button variant="outline" onClick={onSave}>
                        Save settings
                    </Button>
                    <Badge
                        variant={browser.ok ? "default" : "secondary"}
                        className="whitespace-normal text-left"
                    >
                        {browser.ok ? (
                            <IconCheck size={14} className="shrink-0" />
                        ) : (
                            <IconX size={14} className="shrink-0" />
                        )}
                        {browser.text}
                    </Badge>
                </div>

                <div className="text-xs text-muted-foreground">
                    <Tip text="Rough pre-publish estimate from resolution, fps, codec, chroma and rate control. Real bitrate is content-dependent; live figures appear in Publish insights.">
                        <span>Expected bitrate:</span>
                    </Tip>{" "}
                    {estimate ? (
                        <>
                            <span className="text-foreground font-medium">
                                {formatEstimate(estimate)}
                            </span>
                            {" · "}
                            {resLabel}
                            {" · "}
                            {estimate.note}
                        </>
                    ) : (
                        "resolution unavailable - estimate needs the monitor size"
                    )}
                </div>

                <div className="space-y-1">
                    <Tip text="The exact ffmpeg invocation these settings produce.">
                        <span className="text-xs text-muted-foreground">
                            ffmpeg command:
                        </span>
                    </Tip>
                    <pre className="overflow-x-auto rounded-md border bg-muted/50 px-3 py-2 font-mono text-xs whitespace-pre-wrap break-all text-foreground">
                        <code>{cmd}</code>
                    </pre>
                </div>
                {pubError && (
                    <ErrorLog
                        message={pubError}
                        logPath={pubLogPath}
                        onOpenLog={onOpenLog}
                        onOpenLogsFolder={onOpenLogsFolder}
                    />
                )}
            </CardContent>
        </Card>
    );
}
