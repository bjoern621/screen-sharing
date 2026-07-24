import { useMemo, useRef } from "react";
import { Option } from "../../types/stream";
import { useSelectPopup } from "@/components/ui/select";
import Tip from "../Tip/Tip";
import InfoIcon from "../InfoIcon/InfoIcon";

interface OptionRowProps {
    option: Option;
    disabledReason?: string;
}

/**
 * Renders one select option: label plus an optional reference-article icon.
 * A row with a tip (or a disabled reason) carries a tooltip; one without renders
 * just the label. The tooltip anchors to a virtual rect that borrows its
 * horizontal extent from the whole dropdown popup and its vertical extent from
 * this row, so it sits to the left of the entire box while staying level with
 * the option text. Base UI flips right, then above, when the left edge lacks room.
 */
export default function OptionRow({ option, disabledReason }: OptionRowProps) {
    const popup = useSelectPopup();
    const rowRef = useRef<HTMLSpanElement>(null);
    const anchor = useMemo(
        () => ({
            getBoundingClientRect: () => {
                const row = rowRef.current?.getBoundingClientRect();
                const box = popup?.getBoundingClientRect();
                if (!row || !box) return row ?? new DOMRect();
                return new DOMRect(box.left, row.top, box.width, row.height);
            },
        }),
        [popup],
    );

    // A disabled reason appends to the normal tip rather than replacing it, so
    // the option still explains what it does before saying why it is unavailable.
    const unavailable = disabledReason ? `Unavailable: ${disabledReason}` : "";
    const tip = [option.tip, unavailable].filter(Boolean).join("\n\n");
    const content = (
        <>
            <span>{option.label}</span>
            {option.link && <InfoIcon url={option.link} />}
        </>
    );

    if (!tip) {
        return <span className="flex w-full items-center gap-2">{content}</span>;
    }

    return (
        <Tip
            text={tip}
            side="left"
            anchor={anchor}
            triggerRef={rowRef}
            className="flex w-full items-center gap-2"
        >
            {content}
        </Tip>
    );
}
