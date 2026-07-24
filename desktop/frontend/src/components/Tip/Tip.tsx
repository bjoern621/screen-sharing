import { ReactNode, Ref } from "react";
import {
    Tooltip, TooltipContent, TooltipTrigger,
} from "@/components/ui/tooltip";

/** A DOM element or a virtual element (anything with getBoundingClientRect). */
type Anchor = Element | { getBoundingClientRect: () => DOMRect } | null;

interface TipProps {
    text: string;
    side?: "top" | "right" | "bottom" | "left";
    sideOffset?: number;
    /** Position against this element instead of the trigger (e.g. a dropdown popup). */
    anchor?: Anchor;
    /** Ref to the trigger element, e.g. to build a virtual anchor from its rect. */
    triggerRef?: Ref<HTMLSpanElement>;
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
    anchor,
    triggerRef,
    className,
    children,
}: TipProps) {
    return (
        <Tooltip>
            <TooltipTrigger render={<span ref={triggerRef} className={className}>{children}</span>} />
            <TooltipContent side={side} sideOffset={sideOffset} anchor={anchor} className="max-w-sm whitespace-pre-line">
                {text}
            </TooltipContent>
        </Tooltip>
    );
}
