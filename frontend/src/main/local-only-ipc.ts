// IPC stubs for the local-only build (see shared/local-only.ts).
//
// The renderer keeps its cloud, remote-host, feature-build, and update code,
// and some of it invokes these channels without first checking a gate (for
// example useCloudSession asks cloud:getSession on mount). Rather than let
// those calls fail with "No handler registered", main registers these stubs in
// place of the real handlers: each answers with the disabled state and none of
// them touches the network.
import type { IpcMain, IpcMainInvokeEvent } from "electron";
import { LOCAL_ONLY_DISABLED_MESSAGE } from "../shared/local-only";
import type { UpdateInstallResult, UpdateStatus } from "./update-settings";

type IpcRegistrar = Pick<IpcMain, "handle" | "on">;

function disabled(): never {
	throw new Error(LOCAL_ONLY_DISABLED_MESSAGE);
}

/** AO Cloud: always signed out, never signs in, never proxies the control plane. */
export function installLocalOnlyCloudIpc(ipc: IpcRegistrar): void {
	ipc.handle("cloud:getSession", () => null);
	ipc.handle("cloud:signIn", () => undefined);
	ipc.handle("cloud:signOut", () => undefined);
	ipc.handle("cloud:cancelProviderAuth", () => undefined);
	ipc.handle("cloud:connectProviderAuth", disabled);
	ipc.handle("cloud:localAuthAvailable", () => false);
	ipc.handle("cloud:localRegister", disabled);
	ipc.handle("cloud:localLogin", disabled);
	ipc.handle("cloudCp:request", disabled);
	ipc.handle("cloudCp:openStream", disabled);
	ipc.on("cloudCp:closeStream", () => undefined);
	ipc.handle("cloudCp:agentBrowserUrl", () => "");
}

/** Remote hosts: none saved, none can be added or connected. */
export function installLocalOnlyRemotesIpc(ipc: IpcRegistrar): void {
	ipc.handle("remotes:list", () => []);
	ipc.handle("remotes:add", disabled);
	ipc.handle("remotes:update", disabled);
	ipc.handle("remotes:remove", () => undefined);
	ipc.handle("remotes:connect", disabled);
	ipc.handle("remotes:importAccountHost", disabled);
	ipc.handle("remotes:pruneAccountHosts", () => undefined);
	ipc.handle("remotes:issueAccountToken", disabled);
	ipc.handle("remotes:disconnect", () => undefined);
	ipc.handle("remotes:computerName", () => "");
	ipc.handle("remotes:previewUrl", disabled);
	ipc.handle("remotes:agentBrowserUrl", disabled);
	ipc.handle("remotes:resolvePreviewUrl", disabled);
	ipc.handle("remotes:externalUrl", disabled);
}

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
