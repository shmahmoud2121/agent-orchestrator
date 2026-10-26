import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "../ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "../ui/tooltip";
import { apiErrorMessage } from "../../lib/api-client";
import { clientForSessionHost } from "../../lib/host-clients";
import type { components } from "../../../api/schema";
import {
	approvalKey,
	bulkApprovable,
	denyOption,
	optionByKind,
	projectApprovalsQueryKey,
	workerApprovals,
	type ProjectApproval,
} from "./project-approvals";

type WireApproval = components["schemas"]["ProjectApproval"];

async function fetchProjectApprovals(projectId: string, hostId?: string): Promise<ProjectApproval[]> {
	const { data, error } = await clientForSessionHost(hostId).GET("/api/v1/projects/{id}/approvals", {
		params: { path: { id: projectId } },
	});
	if (error) throw error;
	return (data?.approvals ?? []).map(toApproval);
}

function toApproval(item: WireApproval): ProjectApproval {
	return {
		sessionId: item.sessionId,
		requestId: item.requestId,
		sessionKind: item.sessionKind,
		displayName: item.displayName,
		summary: item.summary,
		bulkSafe: item.bulkSafe,
		options: (item.options ?? []).map((option) => ({
			id: option.id,
			kind: option.kind,
			label: option.label,
		})),
	};
}

async function resolveApproval(item: ProjectApproval, decisionId: string, hostId?: string): Promise<void> {
	const { error } = await clientForSessionHost(hostId).POST(
		"/api/v1/sessions/{sessionId}/conversation/approvals/{requestId}/resolve",
		{
			params: { path: { sessionId: item.sessionId, requestId: item.requestId } },
			body: { decisionId },
		},
	);
	if (error) throw error;
}

export function ProjectApprovalBar({
	projectId,
	hostId,
	onOpenSession,
}: {
	projectId?: string;
	hostId?: string;
	onOpenSession: (sessionId: string) => void;
}) {
	const queryClient = useQueryClient();
	const [pending, setPending] = useState<ReadonlySet<string>>(new Set());
	const [errors, setErrors] = useState<Record<string, string>>({});
	const query = useQuery({
		queryKey: projectApprovalsQueryKey(hostId, projectId ?? ""),
		queryFn: () => fetchProjectApprovals(projectId ?? "", hostId),
		enabled: Boolean(projectId),
		// Local updates arrive on the same session event stream as the open chat.
		// A remote host has no faster channel than its existing workspace poll.
		refetchInterval: hostId ? 15_000 : false,
	});

	const decide = async (items: readonly ProjectApproval[], decisionIdFor: (item: ProjectApproval) => string | undefined) => {
		const batch = items.flatMap((item) => {
			const decisionId = decisionIdFor(item);
			return decisionId ? [{ item, decisionId }] : [];
		});
		if (batch.length === 0) return;
		const keys = batch.map(({ item }) => approvalKey(item));
		setPending((current) => new Set([...current, ...keys]));
		setErrors((current) => {
			const next = { ...current };
			for (const key of keys) delete next[key];
			return next;
		});
		const results = await Promise.all(batch.map(async ({ item, decisionId }) => {
			try {
				await resolveApproval(item, decisionId, hostId);
				return { key: approvalKey(item), error: undefined };
			} catch (error) {
				return { key: approvalKey(item), error: apiErrorMessage(error) };
			}
		}));
		setPending((current) => {
			const next = new Set(current);
			for (const key of keys) next.delete(key);
			return next;
		});
		setErrors((current) => {
			const next = { ...current };
			for (const result of results) {
				if (result.error) next[result.key] = result.error;
				else delete next[result.key];
			}
			return next;
		});
		if (results.some((result) => !result.error)) {
			await queryClient.invalidateQueries({ queryKey: projectApprovalsQueryKey(hostId, projectId ?? "") });
		}
	};

	if (!projectId || !query.data) return null;
	return (
		<ProjectApprovalBarView
			items={query.data}
			pending={pending}
			errors={errors}
			onDecide={(item, decisionId) => { void decide([item], () => decisionId); }}
			onApproveSafe={(items) => {
				void decide(items, (item) => optionByKind(item.options, "allow_once")?.id);
			}}
			onOpenSession={onOpenSession}
		/>
	);
}

export function ProjectApprovalBarView({
	items,
	pending,
	errors,
	onDecide,
	onApproveSafe,
	onOpenSession,
}: {
	items: readonly ProjectApproval[];
	pending?: ReadonlySet<string>;
	errors?: Readonly<Record<string, string>>;
	onDecide: (item: ProjectApproval, decisionId: string) => void;
	onApproveSafe: (items: readonly ProjectApproval[]) => void;
	onOpenSession: (sessionId: string) => void;
}) {
	const { t } = useTranslation();
	const workers = workerApprovals(items);
	if (workers.length === 0) return null;
	const safe = bulkApprovable(workers);
	const grouped = workers.length >= 2;
	const bulkBusy = safe.some((item) => pending?.has(approvalKey(item)));
	return (
		<div className="flex flex-col gap-1.5" data-testid="project-approval-bar">
			{grouped ? (
				<div className="flex items-center justify-between gap-2">
					<p className="text-xs text-muted-foreground">{t("chat.approvals.waiting", { count: workers.length })}</p>
					{safe.length === 0 ? null : (
						<SafeApproveButton
							label={safe.length === workers.length
								? t("chat.approvals.approveAll")
								: t("chat.approvals.approveSafe", { count: safe.length })}
							hint={safe.length === workers.length ? undefined : t("chat.approvals.approveSafeHint")}
							disabled={bulkBusy}
							onClick={() => onApproveSafe(safe)}
						/>
					)}
				</div>
			) : null}
			<ul className="flex flex-col gap-1">
				{workers.map((item) => {
					const key = approvalKey(item);
					const busy = pending?.has(key) ?? false;
					const allow = optionByKind(item.options, "allow_once");
					const deny = denyOption(item.options);
					return (
						<li key={key} className="flex flex-col gap-1">
							<div className="flex min-w-0 items-center gap-2">
								<span className="shrink-0 text-xs text-foreground">{item.displayName}</span>
								<span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground" title={item.summary}>
									{item.summary}
								</span>
								<div className="flex shrink-0 items-center gap-1">
									{allow ? (
										<Button type="button" size="sm" disabled={busy} onClick={() => onDecide(item, allow.id)}>
											{t("chat.approvals.approve")}
										</Button>
									) : (
										<Button type="button" size="sm" variant="ghost" onClick={() => onOpenSession(item.sessionId)}>
											{t("chat.approvals.openChat")}
										</Button>
									)}
									{deny ? (
										<Button type="button" size="sm" variant="ghost" disabled={busy} onClick={() => onDecide(item, deny.id)}>
											{t("chat.approvals.deny")}
										</Button>
									) : allow ? (
										<Button type="button" size="sm" variant="ghost" onClick={() => onOpenSession(item.sessionId)}>
											{t("chat.approvals.openChat")}
										</Button>
									) : null}
								</div>
							</div>
							{errors?.[key] ? <p role="alert" className="text-xs text-destructive">{errors[key]}</p> : null}
						</li>
					);
				})}
			</ul>
		</div>
	);
}

function SafeApproveButton({
	label,
	hint,
	disabled,
	onClick,
}: {
	label: string;
	hint?: string;
	disabled?: boolean;
	onClick: () => void;
}) {
	const button = (
		<Button type="button" size="sm" disabled={disabled} onClick={onClick}>
			{label}
		</Button>
	);
	if (!hint) return button;
	return (
		<TooltipProvider delayDuration={0}>
			<Tooltip>
				<TooltipTrigger asChild>
					<span className="inline-flex">{button}</span>
				</TooltipTrigger>
				<TooltipContent>{hint}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
}

export function WorkerApprovalNotice({ onOpen }: { onOpen: () => void }) {
	const { t } = useTranslation();
	return (
		<p className="px-1 pt-1 text-xs text-muted-foreground">
			{t("chat.approvals.waitingInManager")}{" "}
			<button
				type="button"
				className="underline outline-none focus-visible:ring-2 focus-visible:ring-ring"
				onClick={onOpen}
			>
				{t("chat.approvals.openManager")}
			</button>
		</p>
	);
}
