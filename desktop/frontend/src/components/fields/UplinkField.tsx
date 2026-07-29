import { IconGauge, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import Tip from "../Tip/Tip";
import FieldShell from "./FieldShell";

interface UplinkFieldProps {
    value: number;
    measuring: boolean;
    error: string;
    /** Why the speed test cannot run now, empty when it can. The input stays editable
     * either way: a capacity the user knows is a value, not a measurement. */
    blockedReason?: string;
    onChange: (value: number) => void;
    onRemeasure: () => void;
}

/**
 * Uplink capacity input paired with a remeasure button that runs a real upload
 * speed test and writes the result back into the field.
 */
export default function UplinkField({
    value,
    measuring,
    error,
    blockedReason,
    onChange,
    onRemeasure,
}: UplinkFieldProps) {
    const remeasure = (
        <Button
            type="button"
            variant="outline"
            size="sm"
            className="shrink-0"
            disabled={measuring || !!blockedReason}
            onClick={onRemeasure}
        >
            {measuring ? (
                <>
                    <IconLoader2 size={14} className="animate-spin" /> Measuring
                </>
            ) : (
                <>
                    <IconGauge size={14} /> Remeasure
                </>
            )}
        </Button>
    );

    return (
        <FieldShell
            label="Uplink capacity (Mbit/s)"
            labelTip="Your upload capacity. Only used to warn when the stream needs more than your line carries. Remeasure runs a real upload speed test against a public endpoint."
        >
            <div className="flex items-center gap-1.5">
                <Input
                    type="number"
                    value={value}
                    onChange={e => onChange(parseInt(e.target.value, 10) || 0)}
                />
                {blockedReason ? (
                    <Tip text={blockedReason}>{remeasure}</Tip>
                ) : (
                    remeasure
                )}
            </div>
            {error && <span className="text-destructive text-xs">{error}</span>}
        </FieldShell>
    );
}
