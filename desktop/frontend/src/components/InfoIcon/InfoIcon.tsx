import { IconInfoCircle } from "@tabler/icons-react";
import { openExternal } from "../../util/openExternal";

interface InfoIconProps {
    url: string;
}

/**
 * A reference-article icon meant to live inside a select option row. It opens
 * the article in the system browser while swallowing every pointer/keyboard
 * event so Base UI never treats the click as selecting the option.
 */
export default function InfoIcon({ url }: InfoIconProps) {
    return (
        <span
            role="button"
            tabIndex={0}
            aria-label="Open reference article"
            className="ml-auto shrink-0 cursor-pointer text-muted-foreground transition-colors hover:text-primary active:text-primary-active"
            onPointerDown={e => e.stopPropagation()}
            onPointerUp={e => e.stopPropagation()}
            onMouseDown={e => e.stopPropagation()}
            onMouseUp={e => e.stopPropagation()}
            onKeyDown={e => {
                if (e.key === "Enter" || e.key === " ") {
                    e.stopPropagation();
                    e.preventDefault();
                    openExternal(url);
                }
            }}
            onClick={e => {
                e.stopPropagation();
                e.preventDefault();
                openExternal(url);
            }}
        >
            <IconInfoCircle size={15} />
        </span>
    );
}
