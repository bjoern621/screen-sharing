import { IconGauge, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import FieldShell from "./FieldShell";

interface UplinkFieldProps {
    value: number;
    measuring: boolean;
    error: string;
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
    onChange,
    onRemeasure,
}: UplinkFieldProps) {
    return (
        <FieldShell
            label="Uplink capacity (Mbit/s)"
            labelTip="Your upload capacity. Only used to warn when the stream needs more than your line carries. Remeasure runs a real upload speed test against a public endpoint."
        >
            <div className="flex gap-1.5">
                <Input
                    type="number"
                    value={value}
                    onChange={e => onChange(parseInt(e.target.value, 10) || 0)}
                />
                <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="shrink-0"
                    disabled={measuring}
                    onClick={onRemeasure}
                >
                    {measuring ? (
                        <>
                            <IconLoader2 size={14} className="animate-spin" /> measuring
                        </>
                    ) : (
                        <>
                            <IconGauge size={14} /> remeasure
                        </>
                    )}
                </Button>
            </div>
            {error && <span className="text-destructive text-xs">{error}</span>}
        </FieldShell>
    );
}
