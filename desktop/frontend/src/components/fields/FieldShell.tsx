import { ReactNode } from "react";
import { Label } from "@/components/ui/label";
import Tip from "../Tip/Tip";
import InfoIcon from "../InfoIcon/InfoIcon";

interface FieldShellProps {
    label: string;
    /** Omit for a self-explanatory field that needs no tooltip. */
    labelTip?: string;
    /** Reference-article URL; renders an info icon beside the label. */
    labelLink?: string;
    disabledReason?: string;
    children: ReactNode;
}

/**
 * Shared label wrapper for every settings field. A field with a labelTip (or a
 * disabledReason) carries a tooltip; one without renders the bare label. When
 * the control is ignored, the reason appends to the labelTip, so the tooltip
 * explains what the field does before saying why it is ignored.
 */
export default function FieldShell({
    label,
    labelTip,
    labelLink,
    disabledReason,
    children,
}: FieldShellProps) {
    const ignored = disabledReason ? `Ignored: ${disabledReason}` : "";
    const tip = [labelTip, ignored].filter(Boolean).join("\n\n");
    return (
        <div className="flex flex-col gap-1">
            <Label className="flex w-fit items-center gap-1 text-xs text-muted-foreground">
                {tip ? (
                    <Tip text={tip}>
                        <span>{label}</span>
                    </Tip>
                ) : (
                    <span>{label}</span>
                )}
                {labelLink && <InfoIcon url={labelLink} />}
            </Label>
            {children}
        </div>
    );
}
