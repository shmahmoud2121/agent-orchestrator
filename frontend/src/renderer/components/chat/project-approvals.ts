import type { WorkspaceSession } from "../../types/workspace";
import { isArchivedSession } from "../../lib/standalone-archive";

export interface ProjectApprovalOption {
	id: string;
	kind: string;
	label: string;
}

export interface ProjectApproval {
	sessionId: string;
	requestId: string;
	sessionKind: string;
	displayName: string;
	summary: string;
	bulkSafe: boolean;
	options: ProjectApprovalOption[];
}

export function projectApprovalsQueryKey(hostId: string | undefined, projectId: string) {
	return ["project-approvals", hostId ?? "", projectId] as const;
}

export function approvalKey(item: Pick<ProjectApproval, "sessionId" | "requestId">): string {
	return `${item.sessionId}:${item.requestId}`;
}

/** Kind is the only signal. A matching label with a different kind is ignored. */
export function optionByKind(options: readonly ProjectApprovalOption[], kind: string): ProjectApprovalOption | undefined {
	return options.find((option) => option.kind === kind);
}

export function denyOption(options: readonly ProjectApprovalOption[]): ProjectApprovalOption | undefined {
	return optionByKind(options, "reject_once") ?? optionByKind(options, "reject_always");
}

export function workerApprovals(items: readonly ProjectApproval[]): ProjectApproval[] {
	return items.filter((item) => item.sessionKind === "worker");
}

/** Items "Approve N safe" may send. Each one is bulk-safe and has allow_once. */
export function bulkApprovable(items: readonly ProjectApproval[]): ProjectApproval[] {
	return items.filter((item) => item.bulkSafe && optionByKind(item.options, "allow_once"));
}

export type TeamStatusWord = "working" | "needsYou" | "completed" | "failed";

/**
 * Four words for the team card, taken from the session's existing status.
 * A terminal worker is Needs you only when that status is needs_input.
 */
export function teamStatusWord(session: { status?: string; provisionState?: string }): TeamStatusWord {
	if (session.provisionState === "failed" || session.status === "exited") return "failed";
	if (session.status === "working" || session.status === "no_signal") return "working";
	if (session.status === "needs_input") return "needsYou";
	return "completed";
}

export function teamHeaderWorking(sessions: readonly { status?: string; provisionState?: string }[]): boolean {
	return sessions.some((session) => {
		const word = teamStatusWord(session);
		return word === "working" || word === "needsYou";
	});
}

function timestamp(value?: string): number | undefined {
	if (!value) return undefined;
	const parsed = Date.parse(value);
	return Number.isNaN(parsed) ? undefined : parsed;
}

/** Workers in this project created since the orchestrator conversation started, still on the board. */
export function teamCardWorkers(
	sessions: readonly WorkspaceSession[],
	conversationStartedAt?: string,
): WorkspaceSession[] {
	const started = timestamp(conversationStartedAt);
	return sessions
		.filter((session) => session.kind === "worker" && !isArchivedSession(session))
		.filter((session) => {
			if (started === undefined) return true;
			const created = timestamp(session.createdAt);
			return created !== undefined && created >= started;
		})
		.sort((left, right) => (left.createdAt ?? "").localeCompare(right.createdAt ?? "") || left.id.localeCompare(right.id));
}
