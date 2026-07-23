import { IconFileText, IconFolder } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";

interface ErrorLogProps {
    message: string;
    logPath: string;
    onOpenLog: (path: string) => void;
    onOpenLogsFolder: () => void;
}

/**
 * Renders a failure message as monospace text with buttons to open the full run
 * log for that failure and the logs folder.
 */
export default function ErrorLog({
    message,
    logPath,
    onOpenLog,
    onOpenLogsFolder,
}: ErrorLogProps) {
    return (
        <div className="space-y-2">
            <div className="text-destructive text-xs font-mono whitespace-pre-wrap break-words">
                {message}
            </div>
            <div className="flex flex-wrap gap-2">
                {logPath && (
                    <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => onOpenLog(logPath)}
                    >
                        <IconFileText size={14} /> Open log
                    </Button>
                )}
                <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={onOpenLogsFolder}
                >
                    <IconFolder size={14} /> Logs folder
                </Button>
            </div>
        </div>
    );
}
