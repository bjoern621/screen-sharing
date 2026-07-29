import { IconLoader2, IconCpu } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import { EncodeRate } from "../../types/stream";
import {
    EncodeStanding,
    EncodeVerdict,
    formatEncodeRate,
} from "../../util/encodecheck";
import FieldShell from "./FieldShell";

interface EncodeRateFieldProps {
    /** The measured rate, null until a measurement has been taken. */
    rate: EncodeRate | null;
    /** What the rate says about the target, null while there is no rate. */
    verdict: EncodeVerdict | null;
    /** Whether the settings have moved since the rate was measured. */
    stale: boolean;
    measuring: boolean;
    error: string;
    onMeasure: () => void;
}

/** Text colour per standing: red where the target is unreachable, amber where it
 * depends on content or went unmeasured, muted where it is safe. */
const STANDING_CLASS: Record<EncodeStanding, string> = {
    over: "text-destructive",
    unmeasured: "text-amber-600 dark:text-amber-500",
    content: "text-amber-600 dark:text-amber-500",
    ok: "text-muted-foreground",
};

/**
 * The measured encode rate paired with a measure button, and what the figure says
 * about the target frame rate.
 *
 * The uplink field's counterpart: that one bounds what the line carries away from
 * the machine, this one what the machine produces in the first place. It holds no
 * input, unlike that field, because there is nothing here a user knows better than
 * the measurement.
 */
export default function EncodeRateField({
    rate,
    verdict,
    stale,
    measuring,
    error,
    onMeasure,
}: EncodeRateFieldProps) {
    return (
        <FieldShell
            label="Encode capacity (fps)"
            labelTip="How many frames per second this machine encodes at the settings above, measured by running the configured encoder on generated frames of the captured monitor's size. It is a range because encode cost depends on content: the low end is uncorrelated noise, the high end a moving object on a flat field, and a screen sits between them. An encoder that cannot reach the target does not slow the stream down, it discards the frames it cannot take."
        >
            <div className="flex items-center gap-1.5">
                <span className="text-sm tabular-nums">
                    {rate ? formatEncodeRate(rate) : "not measured"}
                </span>
                <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="shrink-0"
                    disabled={measuring}
                    onClick={onMeasure}
                >
                    {measuring ? (
                        <>
                            <IconLoader2 size={14} className="animate-spin" />{" "}
                            Measuring
                        </>
                    ) : (
                        <>
                            <IconCpu size={14} />{" "}
                            {rate ? "Remeasure" : "Measure"}
                        </>
                    )}
                </Button>
            </div>
            {error && <span className="text-destructive text-xs">{error}</span>}
            {!error && stale && (
                <span className="text-muted-foreground text-xs">
                    Measured at other settings. What a frame costs has changed
                    since, so the figure no longer describes this encoder.
                </span>
            )}
            {!error && !stale && verdict && (
                <span className={`text-xs ${STANDING_CLASS[verdict.standing]}`}>
                    {verdict.text}
                </span>
            )}
        </FieldShell>
    );
}
