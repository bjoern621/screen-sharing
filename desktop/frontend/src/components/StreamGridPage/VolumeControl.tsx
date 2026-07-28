import { IconVolume, IconVolumeOff } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import Tip from "../Tip/Tip";

interface VolumeControlProps {
    muted: boolean;
    volume: number;
    onToggleMute: () => void;
    onVolume: (v: number) => void;
}

/** Mute button that reveals a vertical volume slider above it on hover or focus.
 * The slider column animates in from below the button rather than sliding out
 * sideways, keeping the control compact in a crowded tile chrome. */
export default function VolumeControl({
    muted,
    volume,
    onToggleMute,
    onVolume,
}: VolumeControlProps) {
    return (
        <div className="group/vol relative flex items-center">
            <Tip
                text={
                    muted
                        ? "Play this stream's audio again, at the volume the slider holds."
                        : "Silence this stream without disconnecting it. Frames keep arriving while it is muted."
                }
            >
                <Button variant="ghost" size="icon" onClick={onToggleMute}>
                    {muted ? <IconVolumeOff /> : <IconVolume />}
                </Button>
            </Tip>
            <div className="pointer-events-none absolute bottom-full left-1/2 mb-1 -translate-x-1/2 translate-y-1 scale-95 opacity-0 transition-all duration-200 group-focus-within/vol:pointer-events-auto group-focus-within/vol:translate-y-0 group-focus-within/vol:scale-100 group-focus-within/vol:opacity-100 group-hover/vol:pointer-events-auto group-hover/vol:translate-y-0 group-hover/vol:scale-100 group-hover/vol:opacity-100">
                <div className="flex justify-center rounded-lg border bg-background/90 px-2 py-3 shadow-lg backdrop-blur-md">
                    <Slider
                        orientation="vertical"
                        value={[volume]}
                        min={0}
                        max={1}
                        step={0.05}
                        onValueChange={v =>
                            onVolume(Array.isArray(v) ? (v[0] ?? 1) : v)
                        }
                    />
                </div>
            </div>
        </div>
    );
}
