import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AwakeState } from "../hooks/useAwake";
import { AwakeControl } from "./AwakeControl";
import { TooltipProvider } from "./ui/tooltip";

const test = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));
vi.mock("../lib/platform", () => ({ isMacPlatform: () => true }));
vi.mock("../lib/api-client", () => ({
	apiClient: { GET: test.get, POST: test.post },
	apiErrorMessage: (e: unknown) => String(e),
}));

let state: AwakeState;
const mobileCalls = () => [...test.get.mock.calls, ...test.post.mock.calls]
	.filter(([path]) => String(path).startsWith("/api/v1/mobile/"));

beforeEach(() => {
	state = { settings: { lidClosed: true }, sessions: [], rules: [], activeRules: [], active: false, paused: false, capabilities: [{ id: "idle", available: true }] } as unknown as AwakeState;
	test.get.mockReset().mockImplementation(async (path) => path === "/api/v1/awake"
		? { data: structuredClone(state) }
		: { error: { message: "not found" } });
	test.post.mockReset().mockImplementation(async (path, options) => {
		if (path === "/api/v1/awake/sessions") {
			state.sessions.push({ id: "new", scope: "ao", sessionId: "", mode: "indefinite", boundId: "", boundIdentity: "", createdAt: "2026-10-19T00:00:00Z", endReason: "", label: "For AO", lidClosed: false, ...options.body });
			state.active = true;
		} else if (path === "/api/v1/awake/stop") {
			state.sessions = state.sessions.map((s) => ({ ...s, endedAt: "now" }));
			state.active = false;
		} else if (path !== "/api/v1/awake/resume") {
			return { error: { message: "not found" } };
		}
		return { data: structuredClone(state) };
	});
});

// The local-only daemon serves no /api/v1/mobile routes, so Stay Awake must
// work from the desktop awake service alone and never ask for mobile state.
describe("Stay Awake in the local-only build", () => {
	it("turns on and off without any mobile request", async () => {
		const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
		render(<QueryClientProvider client={client}><TooltipProvider delayDuration={0}><AwakeControl /></TooltipProvider></QueryClientProvider>);
		const coffee = () => screen.getByRole("button", { name: "Stay Awake" });
		await waitFor(() => expect(coffee()).toBeEnabled());

		fireEvent.click(coffee());
		await waitFor(() => expect(coffee()).toHaveAttribute("aria-pressed", "true"));
		await waitFor(() => expect(coffee()).toBeEnabled());
		fireEvent.click(coffee());
		await waitFor(() => expect(coffee()).toHaveAttribute("aria-pressed", "false"));

		expect(screen.queryByRole("alert")).toBeNull();
		expect(test.post).toHaveBeenCalledWith("/api/v1/awake/stop");
		expect(mobileCalls()).toEqual([]);
	});
});
