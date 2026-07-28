import { useEffect, useRef, useState } from "react";
import { IconAppWindow, IconFlask, IconLayoutGrid } from "@tabler/icons-react";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { usePlatform } from "./hooks/usePlatform";
import { useEncoders } from "./hooks/useEncoders";
import { useCapabilities } from "./hooks/useCapabilities";
import { useStreamSettings } from "./hooks/useStreamSettings";
import { usePublish } from "./hooks/usePublish";
import { useLive } from "./hooks/useLive";
import { useTestPublishers } from "./hooks/useTestPublishers";
import { useNativeGrid } from "./hooks/useNativeGrid";
import { useUplinkMeasure } from "./hooks/useUplinkMeasure";
import { useMonitors } from "./hooks/useMonitors";
import { useLogs } from "./hooks/useLogs";
import LoadingScreen from "./components/LoadingScreen/LoadingScreen";
import PresetCard from "./components/PresetCard/PresetCard";
import StreamSettingsCard from "./components/StreamSettingsCard/StreamSettingsCard";
import PublishInsightsCard from "./components/PublishInsightsCard/PublishInsightsCard";
import LiveNowCard from "./components/LiveNowCard/LiveNowCard";
import StreamGridPage from "./components/StreamGridPage/StreamGridPage";

/** Test streams published per toggle: fills a near-square grid row and leaves
 * room to compare against a real capture alongside. */
const TEST_STREAM_COUNT = 3;

/**
 * Composition root: wires the state hooks to the presentational cards. All
 * logic lives in hooks/ and util/; this file only distributes it.
 */
export default function App() {
    const platform = usePlatform();
    const encoders = useEncoders();
    const capabilities = useCapabilities();
    const settings = useStreamSettings(platform, encoders, capabilities);
    const publish = usePublish(settings.s);
    const live = useLive();
    const testPub = useTestPublishers();
    const nativeGrid = useNativeGrid();
    const uplink = useUplinkMeasure(settings.update);
    const monitors = useMonitors();
    const logs = useLogs();
    const [webGridOpen, setWebGridOpen] = useState(false);
    const settingsCard = useRef<HTMLDivElement>(null);

    // The native grid's sidebar asks for the settings from a window of its own.
    // The app raises this one; what the raised window shows is this side's half,
    // so the web grid comes off the screen and the form is scrolled to.
    useEffect(
        () =>
            EventsOn("app:show-settings", () => {
                setWebGridOpen(false);
                settingsCard.current?.scrollIntoView({ behavior: "smooth" });
            }),
        []
    );

    if (!settings.s || !settings.deps || !settings.webGrid || !settings.nativeGrid) {
        return <LoadingScreen />;
    }

    // The watch leg every viewer this app opens receives over, the native grid
    // included. Read out here because the narrowing above does not reach into a
    // handler.
    const watchTransport = settings.s.watchTransport;

    // Why the grid window cannot open on the selected leg, empty when it can.
    // A running window closes whatever leg it was opened on, so the reason
    // gates the open and not the close.
    const gridBlocked =
        nativeGrid.running ||
        nativeGrid.transports.length === 0 ||
        nativeGrid.transports.includes(watchTransport)
            ? ""
            : `The native grid receives through a GStreamer pipeline, which has no source for ${watchTransport}. Set "Watch over" to ${nativeGrid.transports.join(" or ")}.`;

    return (
        <TooltipProvider>
            <div className="p-4 space-y-4 max-w-7xl mx-auto">
                <div className="flex items-center justify-between">
                    <h1 className="text-xl font-semibold">screen-sharing</h1>
                    <div className="flex items-center gap-2">
                        <Button
                            variant="outline"
                            size="sm"
                            onClick={() =>
                                testPub.count > 0
                                    ? void testPub.stop()
                                    : void testPub.start(TEST_STREAM_COUNT)
                            }
                        >
                            <IconFlask size={16} />
                            {testPub.count > 0
                                ? "Stop test streams"
                                : "Test streams"}
                        </Button>
                        <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setWebGridOpen(true)}
                        >
                            <IconLayoutGrid size={16} /> Web grid
                        </Button>
                        {/* The window opens on the "Watch over" leg, and receives
                          * through a GStreamer pipeline rather than a player, so a
                          * leg no source element decodes leaves the button inert
                          * rather than opening on something else. */}
                        <Button
                            variant="outline"
                            size="sm"
                            disabled={!!gridBlocked}
                            onClick={() => void nativeGrid.toggle(watchTransport)}
                        >
                            <IconAppWindow size={16} />
                            {nativeGrid.running
                                ? "Close native grid"
                                : "Native grid"}
                        </Button>
                    </div>
                </div>
                {testPub.error && (
                    <p className="text-sm text-destructive whitespace-pre-wrap">
                        {testPub.error}
                    </p>
                )}
                {nativeGrid.error && (
                    <p className="text-sm text-destructive whitespace-pre-wrap">
                        {nativeGrid.error}
                    </p>
                )}

                <PresetCard
                    preset={settings.preset}
                    userPresets={settings.userPresets}
                    publishing={publish.publishing}
                    onApplyPreset={settings.applyPreset}
                    onDeletePreset={settings.deletePreset}
                />

                <div ref={settingsCard}>
                    <StreamSettingsCard
                        s={settings.s}
                        deps={settings.deps}
                        caps={capabilities}
                        transports={settings.transports}
                        engine={settings.engine}
                        monitors={monitors}
                        webGrid={settings.webGrid}
                        nativeGrid={settings.nativeGrid}
                        cmd={settings.cmd}
                        publishing={publish.publishing}
                        pubError={publish.error}
                        pubLogPath={publish.logPath}
                        uplink={uplink}
                        onUpdate={settings.update}
                        onTogglePublish={publish.toggle}
                        onSavePreset={settings.saveAsPreset}
                        onOpenLog={logs.openLog}
                        onOpenLogsFolder={logs.openLogsFolder}
                    />
                </div>

                <PublishInsightsCard
                    stats={publish.stats}
                    avg5={publish.avg5}
                    peak={publish.peak}
                    targetFps={settings.s.fps}
                    uplinkMbps={settings.s.uplinkMbps}
                    publishing={publish.publishing}
                    sampleAgeSec={publish.sampleAgeSec}
                    stale={publish.stale}
                />

                <LiveNowCard
                    live={live.live}
                    watching={live.watching}
                    watchTransports={live.watchTransports}
                    watchTransportsByFormat={live.watchTransportsByFormat}
                    watchTransport={settings.s.watchTransport}
                    connecting={live.connecting}
                    error={live.error}
                    logPath={live.logPath}
                    srtWatchLatencyMs={settings.s.srtWatchLatencyMs}
                    rtspWatchLatencyMs={settings.s.rtspWatchLatencyMs}
                    rtspWatchProtocol={settings.s.rtspWatchProtocol}
                    onToggleWatch={live.toggleWatch}
                    onUpdateWatchTransport={t =>
                        settings.update({ watchTransport: t })
                    }
                    onUpdateSrtWatchLatency={v =>
                        settings.update({ srtWatchLatencyMs: v })
                    }
                    onUpdateRtspWatchLatency={v =>
                        settings.update({ rtspWatchLatencyMs: v })
                    }
                    onUpdateRtspWatchProtocol={v =>
                        settings.update({ rtspWatchProtocol: v })
                    }
                    onOpenLog={logs.openLog}
                    onOpenLogsFolder={logs.openLogsFolder}
                />

                {webGridOpen && (
                    <StreamGridPage
                        paths={live.live?.paths ?? []}
                        s={settings.s}
                        onClose={() => setWebGridOpen(false)}
                    />
                )}
            </div>
        </TooltipProvider>
    );
}
