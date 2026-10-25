// IPC stubs for the local-only build (see shared/local-only.ts).
//
// The local desktop keeps its update UI, but configures no hosted update feed.
import type { IpcMain, IpcMainInvokeEvent } from "electron";
import type { UpdateInstallResult, UpdateStatus } from "./update-settings";

type IpcRegistrar = Pick<IpcMain, "handle" | "on">;

export const LOCAL_ONLY_UPDATE_MESSAGE = "Updates are disabled in this local-only build.";

/** The status every update read and request answers with. */
export function localOnlyUpdateStatus(requestId?: string): UpdateStatus {
	return {
		state: "unsupported",
		message: LOCAL_ONLY_UPDATE_MESSAGE,
		...(requestId === undefined ? {} : { requestId }),
	};
}

/**
 * Auto-update and feature builds: no feed is configured, nothing is checked or
 * downloaded. A renderer request settles at once with an "unsupported" status
 * on updates:status, the same answer an unpackaged build gives.
 */
export function installLocalOnlyUpdateIpc(ipc: IpcRegistrar): void {
	ipc.handle("featureBuilds:list", () => []);
	ipc.handle("updates:getStatus", () => localOnlyUpdateStatus());
	const settle = (event: IpcMainInvokeEvent, requestId?: string): void => {
		event.sender.send("updates:status", localOnlyUpdateStatus(requestId));
	};
	ipc.handle("updates:check", (event, options?: { requestId?: string }) => settle(event, options?.requestId));
	ipc.handle("updates:returnHome", (event, requestId?: string) => settle(event, requestId));
	ipc.handle("updates:download", (event, requestId?: string) => settle(event, requestId));
	ipc.handle("updates:install", (): UpdateInstallResult => undefined);
}
