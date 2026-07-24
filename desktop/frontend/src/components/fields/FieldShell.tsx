import { ReactNode } from "react";
import { Label } from "@/components/ui/label";
import Tip from "../Tip/Tip";

interface FieldShellProps {
    label: string;
    /** Omit for a self-explanatory field that needs no tooltip. */
    labelTip?: string;
    disabledReason?: string;
    children: ReactNode;
}

/**
 * Shared label wrapper for every settings field. A field with a labelTip (or a
 * disabledReason) carries a tooltip; one without renders the bare label. When
 * the control is ignored, the tooltip explains why instead of what it does.
 */
export default function FieldShell({
    label,
    labelTip,
    disabledReason,
    children,
}: FieldShellProps) {
    const tip = disabledReason ? `Ignored: ${disabledReason}` : labelTip;
    return (
        <div className="flex flex-col gap-1">
            <Label className="text-xs text-muted-foreground">
                {tip ? (
                    <Tip text={tip}>
                        <span>{label}</span>
                    </Tip>
                ) : (
                    <span>{label}</span>
                )}
            </Label>
            {children}
        </div>
    );
}
