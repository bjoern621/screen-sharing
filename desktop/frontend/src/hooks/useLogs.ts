import { useCallback } from "react";
import { OpenLog, OpenLogsFolder } from "../../wailsjs/go/app/App";

/**
 * Opens run logs in the OS. openLog opens a single run's log file; openLogsFolder
 * opens the directory holding every run log. Failures are swallowed - the worst
 * case is nothing opening.
 */
export function useLogs() {
    const openLog = useCallback((path: string) => {
        if (path) {
            OpenLog(path).catch(() => {});
        }
    }, []);

    const openLogsFolder = useCallback(() => {
        OpenLogsFolder().catch(() => {});
    }, []);

    return { openLog, openLogsFolder };
}
