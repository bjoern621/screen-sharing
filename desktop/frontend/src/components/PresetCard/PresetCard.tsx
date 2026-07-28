import { IconTrash } from "@tabler/icons-react";
import {
    Select, SelectContent, SelectGroup, SelectItem, SelectLabel,
    SelectSeparator, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Preset } from "../../types/stream";
import { USER_PREFIX, userPresetValue } from "../../hooks/useStreamSettings";
import { PRESET_HINTS, PRESET_LABELS } from "../../util/presets";

interface PresetCardProps {
    preset: string;
    userPresets: Preset[];
    /** Applying a preset mutates the stream settings, which is unsupported
     * mid-stream, so the selector is disabled while a stream is live. */
    publishing: boolean;
    onApplyPreset: (name: string) => void;
    onDeletePreset: (value: string) => void;
}

/** Master preset selector: built-in presets plus the user's saved ones, with a
 * one-line description of the current choice and a delete control for saved
 * presets. */
export default function PresetCard({
    preset,
    userPresets,
    publishing,
    onApplyPreset,
    onDeletePreset,
}: PresetCardProps) {
    const isUserPreset = preset.startsWith(USER_PREFIX);
    const label = (v: string) =>
        v.startsWith(USER_PREFIX) ? v.slice(USER_PREFIX.length) : PRESET_LABELS[v] ?? v;

    return (
        <Card>
            <CardHeader>
                <CardTitle className="text-base">Preset</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-wrap items-center gap-3">
                <Select
                    value={preset}
                    disabled={publishing}
                    onValueChange={(v: string | null) => v && onApplyPreset(v)}
                >
                    <SelectTrigger className="w-72">
                        <SelectValue>{(v: string) => label(v)}</SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                        <SelectGroup>
                            {Object.keys(PRESET_LABELS).map(k => (
                                <SelectItem key={k} value={k}>
                                    {PRESET_LABELS[k]}
                                </SelectItem>
                            ))}
                        </SelectGroup>
                        {userPresets.length > 0 && (
                            <>
                                <SelectSeparator />
                                <SelectGroup>
                                    <SelectLabel>Saved presets</SelectLabel>
                                    {userPresets.map(p => (
                                        <SelectItem
                                            key={p.name}
                                            value={userPresetValue(p.name)}
                                        >
                                            {p.name}
                                        </SelectItem>
                                    ))}
                                </SelectGroup>
                            </>
                        )}
                    </SelectContent>
                </Select>
                {isUserPreset && (
                    <Button
                        variant="ghost"
                        size="icon"
                        aria-label="Delete preset"
                        disabled={publishing}
                        onClick={() => onDeletePreset(preset)}
                    >
                        <IconTrash />
                    </Button>
                )}
                <span className="text-sm text-muted-foreground">
                    {isUserPreset ? "Saved preset" : PRESET_HINTS[preset]}
                </span>
            </CardContent>
        </Card>
    );
}
