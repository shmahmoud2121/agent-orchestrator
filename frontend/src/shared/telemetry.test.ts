import { mkdtemp, readFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, expect, test } from "vitest";
import {
	buildTelemetryBootstrap,
	defaultDataDir,
	loadOrCreateTelemetryInstallId,
	parseDisabledEvents,
	rendererTelemetryEnabled,
} from "./telemetry";

const tempDirs: string[] = [];
const enabledPolicy = { eventsEnabled: true, consentGeneration: "7f80c8a9-ec67-4a16-a067-a444ffcc5cca", updatedAt: "2026-08-28T10:15:30.000Z", acknowledged: true, consentRenewalRequired: false };

afterEach(async () => {
	await Promise.all(
		tempDirs
			.splice(0)
			.map((dir) => import("node:fs/promises").then(({ rm }) => rm(dir, { recursive: true, force: true }))),
	);
});

test("defaultDataDir prefers AO_DATA_DIR", () => {
	expect(defaultDataDir("linux", { AO_DATA_DIR: "/tmp/custom" }, "/home/test")).toBe("/tmp/custom");
});

test("loadOrCreateTelemetryInstallId persists a stable install id", async () => {
	const dir = await mkdtemp(path.join(os.tmpdir(), "ao-telemetry-"));
	tempDirs.push(dir);

	const first = await loadOrCreateTelemetryInstallId(dir);
	const second = await loadOrCreateTelemetryInstallId(dir);
	const stored = (await readFile(path.join(dir, "telemetry_install_id"), "utf8")).trim();

	expect(first).toMatch(/^ins_/);
	expect(second).toBe(first);
	expect(stored).toBe(first);
});

test("buildTelemetryBootstrap returns null when no home dir is available", async () => {
	await expect(buildTelemetryBootstrap({}, "1.2.3", "linux", "", true, enabledPolicy)).resolves.toBeNull();
});

test("renderer telemetry is off in the local-only build, packaged or opted in", () => {
	expect(rendererTelemetryEnabled({}, false)).toBe(false);
	expect(rendererTelemetryEnabled({}, true)).toBe(false);
	expect(rendererTelemetryEnabled({ AO_TELEMETRY_RENDERER: " ON " }, false)).toBe(false);
	expect(rendererTelemetryEnabled({ AO_TELEMETRY_RENDERER: "on" }, true)).toBe(false);
	expect(rendererTelemetryEnabled({ AO_TELEMETRY_RENDERER: "off" }, true)).toBe(false);
});

// A null bootstrap is the switch itself: initTelemetry bails on it, so the
// renderer never constructs a PostHog client at all. The local-only build
// withholds it for every input that used to produce one: a packaged build, an
// explicit dev opt-in, a failure-reporting opt-out, and a deny list.
test("buildTelemetryBootstrap always withholds the bootstrap in the local-only build", async () => {
	const dir = await mkdtemp(path.join(os.tmpdir(), "ao-telemetry-"));
	tempDirs.push(dir);

	await expect(buildTelemetryBootstrap({}, "0.11.2", "linux", dir, false, enabledPolicy)).resolves.toBeNull();
	await expect(buildTelemetryBootstrap({}, "0.11.2", "linux", dir, true, enabledPolicy)).resolves.toBeNull();
	await expect(buildTelemetryBootstrap({ AO_TELEMETRY_RENDERER: "on" }, "0.11.2", "linux", dir, false, enabledPolicy)).resolves.toBeNull();
	await expect(buildTelemetryBootstrap({}, "0.11.2", "linux", dir, true, { ...enabledPolicy, eventsEnabled: false })).resolves.toBeNull();
	await expect(buildTelemetryBootstrap(
		{ AO_TELEMETRY_DISABLED_EVENTS: "ao.v2.app.active, ao.renderer.* ,, " },
		"0.11.2",
		"linux",
		dir,
		true,
		enabledPolicy,
	)).resolves.toBeNull();
	// Withholding the bootstrap must not mint an install id either.
	await expect(readFile(path.join(dir, "telemetry_install_id"), "utf8")).rejects.toThrow();
});

test("parseDisabledEvents treats absent or blank policy as no policy", () => {
	expect(parseDisabledEvents(undefined)).toEqual([]);
	expect(parseDisabledEvents(" , , ")).toEqual([]);
});
