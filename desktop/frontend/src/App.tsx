import { useState } from "react";
import { IconLayoutGrid } from "@tabler/icons-react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { usePlatform } from "./hooks/usePlatform";
import { useEncoders } from "./hooks/useEncoders";
import { useCapabilities } from "./hooks/useCapabilities";
import { useStreamSettings } from "./hooks/useStreamSettings";
import { usePublish } from "./hooks/usePublish";
import { useLive } from "./hooks/useLive";
import { useUplinkMeasure } from "./hooks/useUplinkMeasure";
import { useMonitors } from "./hooks/useMonitors";
import { useLogs } from "./hooks/useLogs";
import LoadingScreen from "./components/LoadingScreen/LoadingScreen";
import PresetCard from "./components/PresetCard/PresetCard";
import StreamSettingsCard from "./components/StreamSettingsCard/StreamSettingsCard";
import PublishInsightsCard from "./components/PublishInsightsCard/PublishInsightsCard";
import LiveNowCard from "./components/LiveNowCard/LiveNowCard";
import StreamGridPage from "./components/StreamGridPage/StreamGridPage";

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
    const uplink = useUplinkMeasure(settings.update);
    const monitors = useMonitors();
    const logs = useLogs();
    const [gridOpen, setGridOpen] = useState(false);

    if (!settings.s || !settings.deps || !settings.browser) {
        return <LoadingScreen />;
    }

    return (
        <TooltipProvider>
            <div className="p-4 space-y-4 max-w-7xl mx-auto">
                <div className="flex items-center justify-between">
                    <h1 className="text-xl font-semibold">screen-sharing</h1>
                    <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setGridOpen(true)}
                    >
                        <IconLayoutGrid size={16} /> Grid view
                    </Button>
                </div>

                <PresetCard
                    preset={settings.preset}
                    userPresets={settings.userPresets}
                    publishing={publish.publishing}
                    onApplyPreset={settings.applyPreset}
                    onDeletePreset={settings.deletePreset}
                />

                <StreamSettingsCard
                    s={settings.s}
                    deps={settings.deps}
                    caps={capabilities}
                    transports={settings.transports}
                    monitors={monitors}
                    browser={settings.browser}
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

                <PublishInsightsCard
                    stats={publish.stats}
                    avg5={publish.avg5}
                    peak={publish.peak}
                    targetFps={settings.s.fps}
                    uplinkMbps={settings.s.uplinkMbps}
                    publishing={publish.publishing}
                />

                <LiveNowCard
                    live={live.live}
                    watching={live.watching}
                    watchTransports={live.watchTransports}
                    connecting={live.connecting}
                    error={live.error}
                    logPath={live.logPath}
                    watchLatencyMs={settings.s.srtWatchLatencyMs}
                    onToggleWatch={live.toggleWatch}
                    onUpdateWatchLatency={v =>
                        settings.update({ srtWatchLatencyMs: v })
                    }
                    onOpenLog={logs.openLog}
                    onOpenLogsFolder={logs.openLogsFolder}
                />

                {gridOpen && (
                    <StreamGridPage
                        paths={live.live?.paths ?? []}
                        s={settings.s}
                        onClose={() => setGridOpen(false)}
                    />
                )}
            </div>
        </TooltipProvider>
    );
}
