import { ReactNode } from "react";
import { Label } from "@/components/ui/label";
import Tip from "../Tip/Tip";

interface FieldShellProps {
    label: string;
    labelTip: string;
    disabledReason?: string;
    children: ReactNode;
}

/**
 * Shared label + tooltip wrapper for every settings field. When the control is
 * ignored for the current settings, the tooltip explains why instead of what
 * the field does.
 */
export default function FieldShell({
    label,
    labelTip,
    disabledReason,
    children,
}: FieldShellProps) {
    return (
        <div className="flex flex-col gap-1">
            <Label className="text-xs text-muted-foreground">
                <Tip text={disabledReason ? `Ignored: ${disabledReason}` : labelTip}>
                    <span>{label}</span>
                </Tip>
            </Label>
            {children}
        </div>
    );
}
