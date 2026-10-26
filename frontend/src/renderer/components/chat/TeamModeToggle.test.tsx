import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { chatFixtureEmpty } from "../../lib/chat-fixture";
import type { WorkspaceSession } from "../../types/workspace";
import { TooltipProvider } from "../ui/tooltip";
import { ChatWorkspace } from "./ChatWorkspace";
import { TeamModeToggle } from "./TeamModeToggle";

const getMock = vi.fn();
const putMock = vi.fn();

vi.mock("../../lib/host-clients", () => ({
	clientForSessionHost: () => ({
		GET: (...args: unknown[]) => getMock(...args),
		PUT: (...args: unknown[]) => putMock(...args),
	}),
}));

const project = {
	id: "proj-1",
	name: "Proj",
	path: "/proj",
	repo: "proj",
	defaultBranch: "main",
	folderMissing: false,
	kind: "single_repo",
	config: { teamMode: true },
};

function renderWithClient(ui: ReactElement) {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	return render(
		<QueryClientProvider client={client}>
			<TooltipProvider>{ui}</TooltipProvider>
		</QueryClientProvider>,
	);
}

const session = {
	id: "sess-1",
	workspaceId: "proj-1",
	workspaceName: "Proj",
	title: "manager",
	provider: "claude-code",
	kind: "orchestrator",
	mode: "chat",
	status: "working",
	updatedAt: "2026-10-26T00:00:00Z",
	activity: { state: "active", lastActivityAt: "2026-10-26T00:00:00Z" },
	prs: [],
} satisfies WorkspaceSession;

describe("TeamModeToggle", () => {
	beforeEach(() => {
		getMock.mockReset().mockImplementation(async (path: string) => {
			if (String(path).includes("approvals")) return { data: { approvals: [] } };
			return { data: { project } };
		});
		putMock.mockReset().mockResolvedValue({ data: { project } });
	});

	it("shows the saved on state and rolls back when saving fails", async () => {
		const user = userEvent.setup();
		putMock.mockResolvedValue({ error: { message: "disk full" } });
		renderWithClient(<TeamModeToggle projectId="proj-1" />);
		const toggle = await screen.findByRole("button", { name: "Team: the manager can hand work to workers" });
		expect(toggle).toHaveAttribute("aria-pressed", "true");
		await user.click(toggle);
		await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("disk full"));
		expect(screen.getByRole("button", { name: "Team: the manager can hand work to workers" })).toHaveAttribute("aria-pressed", "true");
		expect(putMock).toHaveBeenCalledWith("/api/v1/projects/{id}", expect.objectContaining({
			body: expect.objectContaining({ config: expect.objectContaining({ teamMode: false }) }),
		}));
	});

	it("renders in the orchestrator chat and stays out of a worker chat", async () => {
		const snapshot = chatFixtureEmpty;
		const { rerender } = renderWithClient(
			<ChatWorkspace session={session} sessionRole="orchestrator" snapshot={snapshot} onSend={vi.fn()} />,
		);
		expect(await screen.findByRole("button", { name: "Team: the manager can hand work to workers" })).toBeInTheDocument();
		rerender(
			<QueryClientProvider client={new QueryClient()}>
				<TooltipProvider>
					<ChatWorkspace session={{ ...session, kind: "worker" }} sessionRole="worker" snapshot={snapshot} onSend={vi.fn()} />
				</TooltipProvider>
			</QueryClientProvider>,
		);
		expect(screen.queryByRole("button", { name: "Team: the manager can hand work to workers" })).not.toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Team off: the manager works alone" })).not.toBeInTheDocument();
	});
});
