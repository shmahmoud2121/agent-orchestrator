// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it, vi } from "vitest";
import {
	installLocalOnlyUpdateIpc,
	LOCAL_ONLY_UPDATE_MESSAGE,
} from "./local-only-ipc";

type Handler = (event: unknown, ...args: unknown[]) => unknown;

// ipcMain stand-in: records what was registered and lets a test invoke it.
function fakeIpc() {
	const handlers = new Map<string, Handler>();
	const listeners = new Map<string, Handler>();
	const ipc = {
		handle: (channel: string, handler: Handler) => void handlers.set(channel, handler),
		on: (channel: string, handler: Handler) => void listeners.set(channel, handler),
	};
	return {
		ipc: ipc as unknown as Parameters<typeof installLocalOnlyUpdateIpc>[0],
		invoke: async (channel: string, event: unknown = {}, ...args: unknown[]) => {
			const handler = handlers.get(channel);
			if (!handler) throw new Error(`no handler for ${channel}`);
			return handler(event, ...args);
		},
		channels: () => [...handlers.keys(), ...listeners.keys()],
	};
}

// Every channel the preload exposes under these prefixes, so a channel added
// to the renderer bridge later cannot silently go unanswered in this build.
function preloadChannels(prefix: RegExp): string[] {
	const source = readFileSync(fileURLToPath(new URL("../preload.ts", import.meta.url)), "utf8");
	const channels = new Set<string>();
	for (const match of source.matchAll(/ipcRenderer\.(?:invoke|send)\("([^"]+)"/g)) {
		if (prefix.test(match[1])) channels.add(match[1]);
	}
	return [...channels].sort();
}

describe("local-only IPC stubs", () => {
	it("answer every update and feature-build channel the main process would otherwise wire", () => {
		const fake = fakeIpc();
		installLocalOnlyUpdateIpc(fake.ipc);
		const registered = new Set(fake.channels());
		// Still served by main itself: they read local state and never reach a feed.
		const local = new Set(["featureBuilds:getActive", "updates:isPostUpdateRelaunch", "updates:relaunch"]);
		const expected = preloadChannels(/^(updates|featureBuilds):/).filter((channel) => !local.has(channel));
		expect(expected.filter((channel) => !registered.has(channel))).toEqual([]);
	});

	it("report updates as unsupported and settle a request without checking any feed", async () => {
		const fake = fakeIpc();
		installLocalOnlyUpdateIpc(fake.ipc);
		await expect(fake.invoke("updates:getStatus")).resolves.toEqual({ state: "unsupported", message: LOCAL_ONLY_UPDATE_MESSAGE });
		await expect(fake.invoke("featureBuilds:list")).resolves.toEqual([]);
		const send = vi.fn();
		await fake.invoke("updates:check", { sender: { send } }, { requestId: "r1" });
		await fake.invoke("updates:download", { sender: { send } }, "r2");
		expect(send.mock.calls).toEqual([
			["updates:status", { state: "unsupported", message: LOCAL_ONLY_UPDATE_MESSAGE, requestId: "r1" }],
			["updates:status", { state: "unsupported", message: LOCAL_ONLY_UPDATE_MESSAGE, requestId: "r2" }],
		]);
		await expect(fake.invoke("updates:install")).resolves.toBeUndefined();
	});
});
