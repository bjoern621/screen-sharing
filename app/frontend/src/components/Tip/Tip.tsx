import { ReactNode } from "react";
import {
    Tooltip, TooltipContent, TooltipTrigger,
} from "@/components/ui/tooltip";

interface TipProps {
    text: string;
    side?: "top" | "right" | "bottom" | "left";
    sideOffset?: number;
    className?: string;
    children: ReactNode;
}

/**
 * Wraps children in a non-interactive tooltip. Base UI takes the trigger element
 * via the `render` prop (not Radix-style `asChild`).
 */
export default function Tip({
    text,
    side,
    sideOffset,
    className,
    children,
}: TipProps) {
    return (
        <Tooltip>
            <TooltipTrigger render={<span className={className}>{children}</span>} />
            <TooltipContent side={side} sideOffset={sideOffset} className="max-w-sm">
                {text}
            </TooltipContent>
        </Tooltip>
    );
}
