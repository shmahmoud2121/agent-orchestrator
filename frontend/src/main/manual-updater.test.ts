import { describe, expect, it, vi } from "vitest";
import {
	createManualUpdater,
	installManualUpdateIpc,
	manualUpdateErrorMessage,
	MANUAL_UPDATE_DEV_MESSAGE,
	MANUAL_UPDATE_NO_STATE_DIR_MESSAGE,
	MANUAL_UPDATE_UNSUPPORTED_MESSAGE,
	type ManualUpdaterBackend,
	type ManualUpdaterDeps,
} from "./manual-updater";
import type { UpdateStatus } from "./update-settings";

function fakeBackend(overrides: Partial<ManualUpdaterBackend> = {}) {
	const listeners = new Map<string, (arg: never) => void>();
	const backend = {
		app: { get baseCachePath() { return "/os-cache"; } },
		autoDownload: true,
		autoInstallOnAppQuit: true,
		allowPrerelease: true,
		allowDowngrade: true,
		on: vi.fn((event: string, listener: (arg: never) => void) => {
			listeners.set(event, listener);
		}),
		checkForUpdates: vi.fn().mockResolvedValue({ isUpdateAvailable: true, updateInfo: { version: "0.13.7" } }),
		downloadUpdate: vi.fn().mockResolvedValue([]),
		quitAndInstall: vi.fn(),
		...overrides,
	} as ManualUpdaterBackend;
	return { backend, emit: (event: string, arg: unknown) => listeners.get(event)?.(arg as never) };
}

function setup(deps: Partial<ManualUpdaterDeps> = {}, backendOverrides: Partial<ManualUpdaterBackend> = {}) {
	const { backend, emit } = fakeBackend(backendOverrides);
	const sent: UpdateStatus[] = [];
	const updater = createManualUpdater({
		updater: backend,
		isPackaged: true,
		platform: "win32",
		currentVersion: "0.13.0",
		cacheBaseDir: "/home/me/.ao",
		send: (status) => sent.push(status),
		now: () => 1_000,
		...deps,
	});
	return { updater, backend, emit, sent };
}

describe("manual updater", () => {
	it("never downloads or installs on its own and does nothing until asked", () => {
		const { backend, sent } = setup();
		expect(backend.autoDownload).toBe(false);
		expect(backend.autoInstallOnAppQuit).toBe(false);
		expect(backend.allowPrerelease).toBe(false);
		expect(backend.allowDowngrade).toBe(false);
		expect(backend.checkForUpdates).not.toHaveBeenCalled();
		expect(sent).toEqual([]);
		expect((backend as unknown as { app: { baseCachePath: string } }).app.baseCachePath).toBe("/home/me/.ao");
	});

	it("refuses to download when the cache cannot be kept under ~/.ao", async () => {
		for (const deps of [{ cacheBaseDir: null }, {}] as const) {
			const { updater, backend } = setup(deps, "cacheBaseDir" in deps ? {} : ({ app: undefined } as never));
			await updater.check("req");
			expect(backend.checkForUpdates).not.toHaveBeenCalled();
			expect(updater.getStatus()).toMatchObject({ state: "unsupported", message: MANUAL_UPDATE_NO_STATE_DIR_MESSAGE });
		}
	});

	it("reports a newer published version for the requesting check", async () => {
		const { updater, sent } = setup();
		await updater.check("req-1");
		expect(sent.map((s) => s.state)).toEqual(["checking", "available"]);
		expect(sent[1]).toMatchObject({ version: "0.13.7", requestId: "req-1", checkedAt: 1_000 });
	});

	it("reports up to date when nothing newer is published", async () => {
		const { updater } = setup({}, {
			checkForUpdates: vi.fn().mockResolvedValue({ isUpdateAvailable: false, updateInfo: { version: "0.13.0" } }),
		});
		await updater.check("req-1");
		expect(updater.getStatus()).toMatchObject({ state: "not-available", version: "0.13.0", requestId: "req-1" });
	});

	it("turns a missing release into a plain message", async () => {
		const { updater } = setup({}, {
			checkForUpdates: vi.fn().mockRejectedValue(new Error("HttpError: 404 Not Found\nheaders: {...}")),
		});
		await updater.check("req-1");
		expect(updater.getStatus()).toMatchObject({ state: "error", message: "No published version was found. Check again later." });
	});

	it("shares one check between overlapping requests", async () => {
		const { updater, backend } = setup();
		await Promise.all([updater.check("a"), updater.check("b")]);
		expect(backend.checkForUpdates).toHaveBeenCalledTimes(1);
	});

	it("downloads only after a check found a version, then installs on request", async () => {
		const markRelaunch = vi.fn().mockResolvedValue(undefined);
		const { updater, backend, emit, sent } = setup({ markRelaunch });
		await updater.download("early");
		expect(backend.downloadUpdate).not.toHaveBeenCalled();
		await expect(updater.install()).rejects.toThrow(/not ready/);

		await updater.check("c");
		const download = updater.download("d");
		emit("download-progress", { percent: 41.6, transferred: 4, total: 10, bytesPerSecond: 2 });
		await download;
		expect(sent.find((s) => s.state === "downloading" && s.percent === 42)).toMatchObject({ requestId: "d", version: "0.13.7" });
		expect(updater.getStatus()).toMatchObject({ state: "downloaded", version: "0.13.7", staged: { ready: true } });

		await updater.install();
		expect(markRelaunch).toHaveBeenCalledWith("0.13.7");
		expect(backend.quitAndInstall).toHaveBeenCalledWith(false, true);
	});

	it.each([
		["darwin", true, MANUAL_UPDATE_UNSUPPORTED_MESSAGE],
		["linux", true, MANUAL_UPDATE_UNSUPPORTED_MESSAGE],
		["win32", false, MANUAL_UPDATE_DEV_MESSAGE],
	] as const)("stays offline on %s (packaged=%s)", async (platform, isPackaged, message) => {
		const { updater, backend, sent } = setup({ platform, isPackaged });
		await updater.check("req");
		await updater.download("req");
		await updater.install();
		expect(backend.on).not.toHaveBeenCalled();
		expect(backend.checkForUpdates).not.toHaveBeenCalled();
		expect(backend.downloadUpdate).not.toHaveBeenCalled();
		expect(backend.quitAndInstall).not.toHaveBeenCalled();
		expect(sent.at(-1)).toEqual({ state: "unsupported", message, requestId: "req" });
	});

	it("registers every update channel the renderer calls", () => {
		const handle = vi.fn();
		installManualUpdateIpc({ handle, on: vi.fn() } as never, setup().updater);
		expect(handle.mock.calls.map(([channel]) => channel).sort()).toEqual([
			"featureBuilds:list",
			"updates:check",
			"updates:download",
			"updates:getStatus",
			"updates:install",
			"updates:returnHome",
		]);
	});
});

describe("manualUpdateErrorMessage", () => {
	it("keeps network failures and long errors short", () => {
		expect(manualUpdateErrorMessage(new Error("net::ERR_INTERNET_DISCONNECTED"))).toMatch(/internet connection/);
		expect(manualUpdateErrorMessage(new Error("x".repeat(500))).length).toBeLessThanOrEqual(240);
	});
});
