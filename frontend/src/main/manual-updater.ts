// Manual "Check for updates" for the local-only build (see shared/local-only.ts).
//
// Nothing here runs on its own: no timer, no startup check, no telemetry, no
// download the user did not ask for. The feed is contacted only when the user
// presses Check for updates (or Update) in Settings, and it is this build's own
// GitHub releases: electron-updater reads owner/repo from the bundled
// app-update.yml, which forge.config.ts bakes from AO_RELEASE_REPO.
//
// Only Windows releases are published (.github/workflows/windows-installer.yml),
// so every other platform answers "unsupported" without touching the network.
import type { IpcMain, IpcMainInvokeEvent } from "electron";
import type { UpdateInstallResult, UpdateStatus } from "./update-settings";

type IpcRegistrar = Pick<IpcMain, "handle" | "on">;

/** The slice of electron-updater's AppUpdater this module drives. */
export interface ManualUpdaterBackend {
	autoDownload: boolean;
	autoInstallOnAppQuit: boolean;
	allowPrerelease: boolean;
	allowDowngrade: boolean;
	on(event: "download-progress", listener: (progress: DownloadProgress) => void): unknown;
	on(event: "error", listener: (err: Error) => void): unknown;
	checkForUpdates(): Promise<{ isUpdateAvailable?: boolean; updateInfo: { version: string } } | null>;
	downloadUpdate(): Promise<unknown>;
	quitAndInstall(isSilent?: boolean, isForceRunAfter?: boolean): void;
}

/**
 * Points electron-updater's download cache at baseDir instead of the OS cache
 * dir (%LOCALAPPDATA% on Windows): app state lives under ~/.ao only. The
 * updater has no public option for this, so its app adapter's baseCachePath
 * getter is shadowed on the instance. Returns false when the adapter is not
 * there, so the caller can refuse to download anywhere else.
 */
export function pinUpdaterCache(updater: object, baseDir: string): boolean {
	const adapter = (updater as { app?: unknown }).app;
	if (adapter === null || typeof adapter !== "object" || !("baseCachePath" in adapter)) return false;
	Object.defineProperty(adapter, "baseCachePath", { configurable: true, get: () => baseDir });
	return (adapter as { baseCachePath: unknown }).baseCachePath === baseDir;
}

interface DownloadProgress {
	percent: number;
	transferred: number;
	total: number;
	bytesPerSecond: number;
}

export interface ManualUpdaterDeps {
	updater: ManualUpdaterBackend;
	isPackaged: boolean;
	platform: NodeJS.Platform;
	currentVersion: string;
	/** Directory under ~/.ao that holds the downloaded installer; null = unavailable. */
	cacheBaseDir: string | null;
	/** Pushes a status to the shell renderer (updates:status). */
	send: (status: UpdateStatus) => void;
	/** Best-effort post-update relaunch marker for the startup loader. */
	markRelaunch?: (version: string) => Promise<void>;
	now?: () => number;
}

export const MANUAL_UPDATE_UNSUPPORTED_MESSAGE =
	"Updates for this system are not published yet. Install the new version by hand.";
export const MANUAL_UPDATE_DEV_MESSAGE = "Updates are only available in an installed build.";
export const MANUAL_UPDATE_NO_STATE_DIR_MESSAGE = "Updates are unavailable because the Altair data folder could not be found.";
const NO_RELEASE_MESSAGE = "No published version was found. Check again later.";
const MAX_ERROR_LENGTH = 240;

/** Turns an electron-updater failure into one short, plain sentence. */
export function manualUpdateErrorMessage(err: unknown): string {
	const raw = err instanceof Error ? err.message : String(err);
	if (/\b404\b|Cannot find latest\.yml|No published versions/i.test(raw)) return NO_RELEASE_MESSAGE;
	if (/net::ERR_|ENOTFOUND|ECONNREFUSED|ETIMEDOUT|EAI_AGAIN/i.test(raw)) {
		return "Could not reach GitHub. Check your internet connection and try again.";
	}
	const firstLine = raw.split("\n")[0].trim() || "The update check failed.";
	return firstLine.length > MAX_ERROR_LENGTH ? `${firstLine.slice(0, MAX_ERROR_LENGTH - 1)}…` : firstLine;
}

export interface ManualUpdater {
	getStatus(): UpdateStatus;
	check(requestId?: string): Promise<void>;
	download(requestId?: string): Promise<void>;
	install(): Promise<UpdateInstallResult>;
}

export function createManualUpdater(deps: ManualUpdaterDeps): ManualUpdater {
	const { updater } = deps;
	const now = deps.now ?? Date.now;
	const platformSupported = deps.isPackaged && deps.platform === "win32";
	// Fail closed: without a cache dir under ~/.ao, do not download at all.
	const cachePinned = platformSupported && deps.cacheBaseDir !== null && pinUpdaterCache(updater, deps.cacheBaseDir);
	const supported = platformSupported && cachePinned;
	let status: UpdateStatus = supported
		? { state: "idle" }
		: {
			state: "unsupported",
			message: !deps.isPackaged
				? MANUAL_UPDATE_DEV_MESSAGE
				: platformSupported
					? MANUAL_UPDATE_NO_STATE_DIR_MESSAGE
					: MANUAL_UPDATE_UNSUPPORTED_MESSAGE,
		};
	let checkedAt: number | undefined;
	let inFlight: Promise<void> | null = null;
	let activeRequestId: string | undefined;

	const publish = (next: UpdateStatus): void => {
		status = { ...next, ...(checkedAt === undefined ? {} : { checkedAt }) };
		deps.send(status);
	};
	const settleUnsupported = (requestId?: string): void => {
		deps.send({ ...status, ...(requestId === undefined ? {} : { requestId }) });
	};

	if (supported) {
		// Every step is user-initiated: never download or install by itself.
		updater.autoDownload = false;
		updater.autoInstallOnAppQuit = false;
		updater.allowPrerelease = false;
		updater.allowDowngrade = false;
		updater.on("download-progress", (progress) => {
			if (status.state !== "downloading") return;
			publish({
				state: "downloading",
				version: status.version,
				percent: Math.round(progress.percent),
				transferred: progress.transferred,
				total: progress.total,
				bytesPerSecond: progress.bytesPerSecond,
				...(activeRequestId === undefined ? {} : { requestId: activeRequestId }),
			});
		});
		// electron-updater emits "error" alongside each rejected promise below,
		// which reports it; a listener must exist or the emit would throw.
		updater.on("error", (err) => console.warn("[manual-updater]", err.message));
	}

	const serialize = (run: () => Promise<void>): Promise<void> => {
		if (inFlight) return inFlight;
		inFlight = run().finally(() => {
			inFlight = null;
			activeRequestId = undefined;
		});
		return inFlight;
	};

	return {
		getStatus: () => status,

		check(requestId) {
			if (!supported) {
				settleUnsupported(requestId);
				return Promise.resolve();
			}
			// A downloaded build stays ready to install; re-checking would only
			// repeat the same answer.
			if (status.state === "downloaded" || status.state === "downloading") {
				deps.send({ ...status, ...(requestId === undefined ? {} : { requestId }) });
				return Promise.resolve();
			}
			return serialize(async () => {
				activeRequestId = requestId;
				const tag = requestId === undefined ? {} : { requestId };
				publish({ state: "checking", ...tag });
				try {
					const result = await updater.checkForUpdates();
					checkedAt = now();
					const version = result?.updateInfo.version;
					if (result && result.isUpdateAvailable !== false && version && version !== deps.currentVersion) {
						publish({ state: "available", version, ...tag });
					} else {
						publish({ state: "not-available", version: deps.currentVersion, ...tag });
					}
				} catch (err) {
					publish({ state: "error", message: manualUpdateErrorMessage(err), ...tag });
				}
			});
		},

		download(requestId) {
			if (!supported) {
				settleUnsupported(requestId);
				return Promise.resolve();
			}
			if (status.state !== "available") {
				deps.send({ ...status, ...(requestId === undefined ? {} : { requestId }) });
				return Promise.resolve();
			}
			const version = status.version;
			return serialize(async () => {
				activeRequestId = requestId;
				const tag = requestId === undefined ? {} : { requestId };
				publish({ state: "downloading", version, ...tag });
				try {
					await updater.downloadUpdate();
					const stagedAt = now();
					publish({
						state: "downloaded",
						version,
						stagedAt,
						staged: { version, stagedAt, escalated: false, ready: true },
						...tag,
					});
				} catch (err) {
					publish({ state: "error", message: manualUpdateErrorMessage(err), ...tag });
				}
			});
		},

		async install() {
			if (!supported) return;
			if (status.state !== "downloaded" || !status.version) {
				throw new Error("The update is not ready to install. Check for updates again.");
			}
			if (deps.markRelaunch) {
				// Best-effort and time-bounded: the marker only drives loader copy.
				await Promise.race([
					deps.markRelaunch(status.version).catch((err) => {
						console.warn("failed to write post-update relaunch marker:", err);
					}),
					new Promise<void>((resolve) => setTimeout(resolve, 750)),
				]);
			}
			updater.quitAndInstall(false, true);
		},
	};
}

/**
 * Registers the update IPC for the local-only build. Feature builds and
 * channel switching stay off: there is one channel, this build's own releases.
 */
export function installManualUpdateIpc(ipc: IpcRegistrar, updater: ManualUpdater): void {
	ipc.handle("featureBuilds:list", () => []);
	ipc.handle("updates:getStatus", () => updater.getStatus());
	ipc.handle("updates:check", (_event: IpcMainInvokeEvent, options?: { requestId?: string }) =>
		updater.check(options?.requestId),
	);
	ipc.handle("updates:returnHome", (_event: IpcMainInvokeEvent, requestId?: string) => updater.check(requestId));
	ipc.handle("updates:download", (_event: IpcMainInvokeEvent, requestId?: string) => updater.download(requestId));
	ipc.handle("updates:install", (): Promise<UpdateInstallResult> => updater.install());
}
