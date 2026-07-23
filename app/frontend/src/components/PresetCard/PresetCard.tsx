import {
    Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PRESET_HINTS, PRESET_LABELS } from "../../util/presets";

interface PresetCardProps {
    preset: string;
    onApplyPreset: (name: string) => void;
}

/** Master preset selector with a one-line description of the current choice. */
export default function PresetCard({ preset, onApplyPreset }: PresetCardProps) {
    return (
        <Card>
            <CardHeader>
                <CardTitle className="text-base">Preset</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-wrap items-center gap-3">
                <Select
                    value={preset}
                    onValueChange={(v: string | null) => v && onApplyPreset(v)}
                >
                    <SelectTrigger className="w-72">
                        <SelectValue>{(v: string) => PRESET_LABELS[v] ?? v}</SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                        {Object.keys(PRESET_LABELS).map(k => (
                            <SelectItem key={k} value={k}>
                                {PRESET_LABELS[k]}
                            </SelectItem>
                        ))}
                    </SelectContent>
                </Select>
                <span className="text-sm text-muted-foreground">
                    {PRESET_HINTS[preset]}
                </span>
            </CardContent>
        </Card>
    );
}
