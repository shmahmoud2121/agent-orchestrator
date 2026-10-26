import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Users } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { components } from "../../../api/schema";
import { apiErrorMessage } from "../../lib/api-client";
import { clientForSessionHost } from "../../lib/host-clients";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip";

type Project = components["schemas"]["Project"];

function teamModeKey(projectId: string, hostId?: string) {
	return ["project-team-mode", hostId ?? "", projectId] as const;
}

/** Saved team mode. An omitted value is on, matching the daemon default. */
export function teamModeEnabled(project: Project | undefined): boolean {
	return project?.config?.teamMode !== false;
}

export function TeamModeToggle({ projectId, hostId }: { projectId?: string; hostId?: string }) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const [pending, setPending] = useState<boolean | null>(null);
	const [error, setError] = useState<string>();
	const query = useQuery({
		queryKey: teamModeKey(projectId ?? "", hostId),
		enabled: Boolean(projectId),
		queryFn: async () => {
			const { data, error: loadError } = await clientForSessionHost(hostId).GET("/api/v1/projects/{id}", {
				params: { path: { id: projectId ?? "" } },
			});
			if (loadError) throw loadError;
			const project = data?.project;
			if (!project || !("defaultBranch" in project)) throw new Error("Missing project");
			return project;
		},
	});
	if (!projectId || !query.data) return null;
	const saved = teamModeEnabled(query.data);
	const shown = pending ?? saved;
	const label = shown ? t("chat.team.modeOn") : t("chat.team.modeOff");
	const toggle = async () => {
		const next = !shown;
		const project = query.data;
		if (!project) return;
		setError(undefined);
		setPending(next);
		const { error: saveError } = await clientForSessionHost(hostId).PUT("/api/v1/projects/{id}", {
			params: { path: { id: projectId } },
			body: {
				displayName: project.name,
				config: { ...project.config, teamMode: next },
			},
		});
		if (saveError) {
			setPending(null);
			setError(apiErrorMessage(saveError, "Could not save team mode."));
			return;
		}
		queryClient.setQueryData<Project>(teamModeKey(projectId, hostId), {
			...project,
			config: { ...project.config, teamMode: next },
		});
		setPending(null);
	};
	return (
		<div className="flex flex-col">
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						type="button"
						variant="ghost"
						size="icon-sm"
						aria-pressed={shown}
						aria-label={label}
						disabled={pending !== null}
						onClick={() => { void toggle(); }}
						className={cn("size-7 shrink-0 rounded-full p-0", shown ? "text-foreground" : "text-muted-foreground")}
					>
						<Users aria-hidden="true" className="size-3.5" />
					</Button>
				</TooltipTrigger>
				<TooltipContent side="bottom">{label}</TooltipContent>
			</Tooltip>
			{error ? (
				<p role="alert" className="px-1 text-xs leading-snug text-destructive">{error}</p>
			) : null}
		</div>
	);
}
