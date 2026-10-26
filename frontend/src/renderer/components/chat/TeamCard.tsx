import { QueryClientContext, useQuery } from "@tanstack/react-query";
import { ChevronRight, Lock } from "lucide-react";
import { useContext, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { agentLabel } from "../../lib/agent-options";
import { useRemoteProjectQuery, workspaceQueryOptions } from "../../hooks/useWorkspaceQuery";
import { newestActiveOrchestrator, type WorkspaceSession } from "../../types/workspace";
import { AgentAvatar } from "../AgentAvatar";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "../ui/tooltip";
import { teamCardWorkers, teamHeaderWorking, teamStatusWord, type TeamStatusWord } from "./project-approvals";

/** Chat unit tests render the workspace without a QueryClient. The team card and
 * approval bar stay unmounted there; the app shell always has a client. */
export function useHasQueryClient(): boolean {
	return useContext(QueryClientContext) != null;
}

export function WhenQueryClient({ children }: { children: ReactNode }) {
	const hasClient = useHasQueryClient();
	if (!hasClient) return null;
	return children;
}

function localWorkspaceOptions() {
	try {
		return workspaceQueryOptions;
	} catch {
		// Some unit tests replace this module and only stub the hooks they render.
		return { queryKey: ["workspaces"] as const, queryFn: async () => [] };
	}
}

export function useProjectSessions(projectId?: string, hostId?: string): WorkspaceSession[] {
	const local = useQuery({
		...localWorkspaceOptions(),
		enabled: Boolean(projectId) && !hostId,
	});
	const remote = useRemoteProjectQuery(hostId ?? "", hostId ? (projectId ?? "") : "");
	if (hostId) return remote.data?.sessions ?? [];
	return local.data?.find((workspace) => workspace.id === projectId)?.sessions ?? [];
}

export function useManagerOrchestrator(session?: WorkspaceSession, role?: string): WorkspaceSession | undefined {
	const sessions = useProjectSessions(session?.workspaceId, session?.hostId);
	if (role !== "worker") return undefined;
	return newestActiveOrchestrator(sessions);
}

export function TeamCard({
	projectId,
	hostId,
	conversationStartedAt,
	onOpenSession,
	onOpenProject,
}: {
	projectId?: string;
	hostId?: string;
	conversationStartedAt?: string;
	onOpenSession: (sessionId: string) => void;
	onOpenProject: () => void;
}) {
	const sessions = useProjectSessions(projectId, hostId);
	const workers = teamCardWorkers(sessions, conversationStartedAt);
	if (!projectId || workers.length === 0) return null;
	return (
		<TeamCardView
			workers={workers}
			onOpenSession={onOpenSession}
			onOpenProject={onOpenProject}
		/>
	);
}

const statusClass: Record<TeamStatusWord, string> = {
	working: "text-status-working",
	needsYou: "text-status-needs-you",
	completed: "text-status-ready",
	failed: "text-status-exited",
};

export function TeamCardView({
	workers,
	onOpenSession,
	onOpenProject,
}: {
	workers: readonly WorkspaceSession[];
	onOpenSession: (sessionId: string) => void;
	onOpenProject: () => void;
}) {
	const { t } = useTranslation();
	if (workers.length === 0) return null;
	const working = teamHeaderWorking(workers);
	return (
		<section className="rounded-lg border border-border bg-card" data-testid="team-card">
			<header className="flex items-center gap-2 px-3 py-2">
				<span
					aria-hidden="true"
					className={working ? "size-2 shrink-0 rounded-full bg-status-working" : "size-2 shrink-0 rounded-full bg-status-ready"}
				/>
				<h2 className="min-w-0 flex-1 text-sm text-foreground">
					{working ? t("chat.team.working") : t("chat.team.complete")}
				</h2>
				<button
					type="button"
					className="text-xs text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
					onClick={onOpenProject}
				>
					{t("chat.team.viewWorkers")}
				</button>
			</header>
			<ul>
				{workers.map((worker) => {
					const word = teamStatusWord(worker);
					const model = worker.model?.trim();
					const harness = agentLabel(worker.provider);
					return (
						<li key={worker.id}>
							<button
								type="button"
								className="flex w-full items-center gap-2 px-3 py-2 text-left outline-none hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring"
								onClick={() => onOpenSession(worker.id)}
							>
								<AgentAvatar provider={worker.provider} className="size-5" decorative />
								<span className="min-w-0 flex-1">
									<span className="block truncate text-sm text-foreground">{worker.title}</span>
									<span className="block truncate text-xs text-muted-foreground">
										{model ? `${harness} · ${model}` : harness}
									</span>
								</span>
								<span className={`shrink-0 text-xs ${statusClass[word]}`}>{t(statusKey[word])}</span>
								<SandboxMark sandboxed={worker.sandboxed === true} />
								<ChevronRight aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground" />
							</button>
						</li>
					);
				})}
			</ul>
		</section>
	);
}

const statusKey: Record<TeamStatusWord, "chat.team.workingStatus" | "chat.team.needsYou" | "chat.team.completed" | "chat.team.failed"> = {
	working: "chat.team.workingStatus",
	needsYou: "chat.team.needsYou",
	completed: "chat.team.completed",
	failed: "chat.team.failed",
};

function SandboxMark({ sandboxed }: { sandboxed: boolean }) {
	const { t } = useTranslation();
	if (sandboxed) {
		return (
			<TooltipProvider delayDuration={0}>
				<Tooltip>
					<TooltipTrigger asChild>
						<span className="inline-flex shrink-0 text-muted-foreground">
							<Lock aria-hidden="true" className="size-3" />
							<span className="sr-only">{t("chat.team.sandboxed")}</span>
						</span>
					</TooltipTrigger>
					<TooltipContent>{t("chat.team.sandboxed")}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}
	return (
		<TooltipProvider delayDuration={0}>
			<Tooltip>
				<TooltipTrigger asChild>
					<span className="shrink-0 text-xs text-warning">{t("chat.team.notSandboxed")}</span>
				</TooltipTrigger>
				<TooltipContent>{t("chat.team.notSandboxedHint")}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
}
