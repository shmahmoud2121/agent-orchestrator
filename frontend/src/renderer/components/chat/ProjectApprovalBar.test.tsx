import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ConversationActivity } from "../../types/conversation";
import type { WorkspaceSession } from "../../types/workspace";
import { ApprovalCard } from "./ChatTimelineItems";
import { ProjectApprovalBarView, WorkerApprovalNotice } from "./ProjectApprovalBar";
import { TeamCardView } from "./TeamCard";
import {
	bulkApprovable,
	denyOption,
	optionByKind,
	teamCardWorkers,
	teamStatusWord,
	type ProjectApproval,
} from "./project-approvals";

function approval(partial: Partial<ProjectApproval> & Pick<ProjectApproval, "sessionId" | "requestId">): ProjectApproval {
	return {
		sessionKind: "worker",
		displayName: partial.sessionId,
		summary: "edit file",
		bulkSafe: true,
		options: [
			{ id: "allow", kind: "allow_once", label: "Always allow" },
			{ id: "always", kind: "allow_always", label: "Allow once" },
			{ id: "reject", kind: "reject_once", label: "No" },
		],
		...partial,
	};
}

function worker(partial: Partial<WorkspaceSession> & Pick<WorkspaceSession, "id">): WorkspaceSession {
	return {
		workspaceId: "project",
		workspaceName: "Project",
		title: partial.id,
		provider: "claude-code",
		prs: [],
		updatedAt: "2026-10-26T12:00:00Z",
		kind: "worker",
		status: "idle",
		createdAt: "2026-10-26T11:00:00Z",
		...partial,
	};
}

const pendingActivity: ConversationActivity = {
	kind: "activity",
	id: "act",
	sequence: 1,
	revision: 1,
	activityKind: "approval",
	status: "pending",
	summary: "Run git status",
	requestId: "req",
	decisions: [{ id: "allow", kind: "allow_once", label: "Allow once" }],
	createdAt: "2026-10-26T12:00:00Z",
};

describe("approval decisions", () => {
	it("uses kind and ignores a label that says the opposite", () => {
		const options = [
			{ id: "mislabeled", kind: "allow_always", label: "Allow once" },
			{ id: "once", kind: "allow_once", label: "Always allow" },
			{ id: "no", kind: "reject_always", label: "Deny once" },
		];
		expect(optionByKind(options, "allow_once")?.id).toBe("once");
		expect(denyOption(options)?.id).toBe("no");
		expect(optionByKind(options, "allow_always")?.id).toBe("mislabeled");
	});

	it("counts only bulk-safe items that offer allow_once", () => {
		const items = [
			approval({ sessionId: "safe", requestId: "1" }),
			approval({ sessionId: "shell", requestId: "2", bulkSafe: false }),
			approval({
				sessionId: "no-once",
				requestId: "3",
				options: [{ id: "always", kind: "allow_always", label: "Allow once" }],
			}),
		];
		expect(bulkApprovable(items).map((item) => item.sessionId)).toEqual(["safe"]);
	});
});

describe("ProjectApprovalBarView", () => {
	it("shows one worker request with Approve and Deny", () => {
		const onDecide = vi.fn();
		render(
			<ProjectApprovalBarView
				items={[approval({ sessionId: "worker-1", requestId: "req", displayName: "Ada", summary: "src/main.go" })]}
				onDecide={onDecide}
				onApproveSafe={vi.fn()}
				onOpenSession={vi.fn()}
			/>,
		);
		expect(screen.getByText("Ada")).toBeInTheDocument();
		expect(screen.getByText("src/main.go")).toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: "Approve" }));
		expect(onDecide).toHaveBeenCalledWith(expect.objectContaining({ requestId: "req" }), "allow");
		fireEvent.click(screen.getByRole("button", { name: "Deny" }));
		expect(onDecide).toHaveBeenLastCalledWith(expect.objectContaining({ requestId: "req" }), "reject");
		expect(screen.queryByRole("button", { name: "Approve all" })).not.toBeInTheDocument();
	});

	it("approves only the safe allow_once items from the group", () => {
		const onApproveSafe = vi.fn();
		const shell = approval({ sessionId: "shell", requestId: "2", bulkSafe: false, displayName: "Bea", summary: "rm -rf" });
		const safe = approval({ sessionId: "safe", requestId: "1", displayName: "Ada", summary: "src/a.go" });
		render(
			<ProjectApprovalBarView
				items={[safe, shell, approval({ sessionId: "orch", requestId: "9", sessionKind: "orchestrator" })]}
				onDecide={vi.fn()}
				onApproveSafe={onApproveSafe}
				onOpenSession={vi.fn()}
			/>,
		);
		expect(screen.getByText("2 approvals waiting")).toBeInTheDocument();
		expect(screen.queryByText("orch")).not.toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: "Approve 1 safe" }));
		expect(onApproveSafe).toHaveBeenCalledWith([safe]);
	});

	it("says Approve all when every waiting item is safe", () => {
		const onApproveSafe = vi.fn();
		const items = [
			approval({ sessionId: "a", requestId: "1" }),
			approval({ sessionId: "b", requestId: "2" }),
		];
		render(
			<ProjectApprovalBarView items={items} onDecide={vi.fn()} onApproveSafe={onApproveSafe} onOpenSession={vi.fn()} />,
		);
		fireEvent.click(screen.getByRole("button", { name: "Approve all" }));
		expect(onApproveSafe).toHaveBeenCalledWith(items);
	});

	it("keeps the line and shows the error when resolve fails", () => {
		render(
			<ProjectApprovalBarView
				items={[approval({ sessionId: "worker-1", requestId: "req", displayName: "Ada" })]}
				errors={{ "worker-1:req": "The agent is gone." }}
				onDecide={vi.fn()}
				onApproveSafe={vi.fn()}
				onOpenSession={vi.fn()}
			/>,
		);
		expect(screen.getByText("Ada")).toBeInTheDocument();
		expect(screen.getByRole("alert")).toHaveTextContent("The agent is gone.");
	});

	it("hides when the only pending request is the manager's", () => {
		const { container } = render(
			<ProjectApprovalBarView
				items={[approval({ sessionId: "orch", requestId: "1", sessionKind: "orchestrator" })]}
				onDecide={vi.fn()}
				onApproveSafe={vi.fn()}
				onOpenSession={vi.fn()}
			/>,
		);
		expect(container).toBeEmptyDOMElement();
	});

	it("opens the chat when there is no allow_once and never sends allow_always", () => {
		const onDecide = vi.fn();
		const onOpenSession = vi.fn();
		render(
			<ProjectApprovalBarView
				items={[approval({
					sessionId: "codex",
					requestId: "req",
					displayName: "Codex",
					options: [
						{ id: "always", kind: "allow_always", label: "Allow once" },
						{ id: "reject-always", kind: "reject_always", label: "Stop" },
					],
				})]}
				onDecide={onDecide}
				onApproveSafe={vi.fn()}
				onOpenSession={onOpenSession}
			/>,
		);
		expect(screen.queryByRole("button", { name: "Approve" })).not.toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: "Open chat" }));
		expect(onOpenSession).toHaveBeenCalledWith("codex");
		fireEvent.click(screen.getByRole("button", { name: "Deny" }));
		expect(onDecide).toHaveBeenCalledWith(expect.objectContaining({ sessionId: "codex" }), "reject-always");
		expect(onDecide).not.toHaveBeenCalledWith(expect.anything(), "always");
	});

	it("hides Deny when the provider offers no rejection", () => {
		const onOpenSession = vi.fn();
		render(
			<ProjectApprovalBarView
				items={[approval({
					sessionId: "codex",
					requestId: "req",
					options: [{ id: "allow", kind: "allow_once", label: "Yes" }],
				})]}
				onDecide={vi.fn()}
				onApproveSafe={vi.fn()}
				onOpenSession={onOpenSession}
			/>,
		);
		expect(screen.queryByRole("button", { name: "Deny" })).not.toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: "Open chat" }));
		expect(onOpenSession).toHaveBeenCalledWith("codex");
	});

	it("prefers reject_once over reject_always", () => {
		expect(denyOption([
			{ id: "always", kind: "reject_always", label: "Always deny" },
			{ id: "once", kind: "reject_once", label: "No" },
		])?.id).toBe("once");
	});
});

describe("worker approval handoff", () => {
	it("shows the request without decisions and links to the manager chat", () => {
		const onDecide = vi.fn();
		const onOpen = vi.fn();
		render(
			<>
				<ApprovalCard activity={pendingActivity} onDecide={onDecide} embedded readOnly />
				<WorkerApprovalNotice onOpen={onOpen} />
			</>,
		);
		expect(screen.getByText("Run git status")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /Allow once/ })).not.toBeInTheDocument();
		expect(screen.getByText(/Waiting for approval in the manager chat/)).toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: "Open manager chat" }));
		expect(onOpen).toHaveBeenCalledOnce();
		expect(onDecide).not.toHaveBeenCalled();
	});
});

describe("TeamCardView", () => {
	it("says Team working while any worker is working or needs the person", () => {
		render(
			<TeamCardView
				workers={[
					worker({ id: "a", title: "Ada", status: "working", model: "opus", sandboxed: true }),
					worker({ id: "b", title: "Bea", status: "needs_input", mode: "tui", sandboxed: false }),
				]}
				onOpenSession={vi.fn()}
				onOpenProject={vi.fn()}
			/>,
		);
		expect(screen.getByRole("heading", { name: "Team working" })).toBeInTheDocument();
		expect(screen.getByText("Working")).toBeInTheDocument();
		expect(screen.getByText("Needs you")).toBeInTheDocument();
		expect(screen.getByText("Claude Code · opus")).toBeInTheDocument();
		expect(screen.getByText("Not sandboxed")).toBeInTheDocument();
	});

	it("says Team complete and shows the sandbox lock when everyone is done", () => {
		render(
			<TeamCardView
				workers={[worker({ id: "a", title: "Ada", status: "idle", sandboxed: true })]}
				onOpenSession={vi.fn()}
				onOpenProject={vi.fn()}
			/>,
		);
		expect(screen.getByRole("heading", { name: "Team complete" })).toBeInTheDocument();
		expect(screen.getByText("Completed")).toBeInTheDocument();
		expect(screen.getByText("Sandboxed")).toBeInTheDocument();
		expect(screen.queryByText("·")).not.toBeInTheDocument();
	});

	it("opens the worker and the project board", () => {
		const onOpenSession = vi.fn();
		const onOpenProject = vi.fn();
		render(
			<TeamCardView
				workers={[worker({ id: "worker-1", title: "Ada", status: "exited" })]}
				onOpenSession={onOpenSession}
				onOpenProject={onOpenProject}
			/>,
		);
		expect(screen.getByText("Failed")).toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: /Ada/ }));
		expect(onOpenSession).toHaveBeenCalledWith("worker-1");
		fireEvent.click(screen.getByRole("button", { name: "View workers ›" }));
		expect(onOpenProject).toHaveBeenCalledOnce();
	});

	it("maps terminal needs_input to Needs you and leaves other terminal workers alone", () => {
		expect(teamStatusWord({ status: "needs_input" })).toBe("needsYou");
		expect(teamStatusWord({ status: "working" })).toBe("working");
		expect(teamStatusWord({ status: "no_signal" })).toBe("working");
		expect(teamStatusWord({ status: "idle" })).toBe("completed");
		expect(teamStatusWord({ status: "merged" })).toBe("completed");
		expect(teamStatusWord({ provisionState: "failed", status: "idle" })).toBe("failed");
	});

	it("keeps workers created since the conversation started and drops archived ones", () => {
		const sessions = [
			worker({ id: "before", createdAt: "2026-10-26T09:00:00Z" }),
			worker({ id: "after", createdAt: "2026-10-26T11:00:00Z" }),
			worker({ id: "archived", createdAt: "2026-10-26T12:00:00Z", isTerminated: true }),
			worker({ id: "orch", kind: "orchestrator", createdAt: "2026-10-26T12:00:00Z" }),
		];
		expect(teamCardWorkers(sessions, "2026-10-26T10:00:00Z").map((session) => session.id)).toEqual(["after"]);
	});
});
