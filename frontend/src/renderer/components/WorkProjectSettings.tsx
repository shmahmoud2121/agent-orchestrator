import { FileText } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { apiClient, apiErrorCode, apiErrorMessage } from "../lib/api-client";
import { clientForHost } from "../lib/host-clients";
import { aoBridge } from "../lib/bridge";
import { workspaceQueryKey, workspaceQueryKeyForHost } from "../hooks/useWorkspaceQuery";
import { sessionIsActive, type WorkspaceSession } from "../types/workspace";
import { Button } from "./ui/button";
import { Input } from "./ui/input";

type Project = components["schemas"]["Project"];

function baseName(path: string): string {
	const parts = path.split(/[\\/]/).filter(Boolean);
	return parts[parts.length - 1] ?? path;
}

function fieldFor(code: string): "sources" | "results" | "form" {
	if (code === "WORK_SOURCE_REQUIRED" || code === "WORK_SOURCE_MISSING" || code === "WORK_SOURCE_NOT_ABSOLUTE" || code === "WORK_SOURCE_TOO_MANY") return "sources";
	if (code === "WORK_RESULTS_NOT_ABSOLUTE" || code === "WORK_RESULTS_CREATE" || code === "WORK_RESULTS_LOCKED") return "results";
	if (code === "WORK_PATH_OVERLAP" || code === "WORK_PATH_UNDER_AO") return "form";
	return "form";
}

export function WorkProjectSettings({
	projectId,
	project,
	sessions,
	hostId,
}: {
	projectId: string;
	project: Project;
	sessions: WorkspaceSession[];
	hostId?: string;
}) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const stored = project.config?.work;
	const [sources, setSources] = useState(stored?.sources ?? []);
	const [resultsDir, setResultsDir] = useState(stored?.resultsDir ?? project.path);
	const [busy, setBusy] = useState(false);
	const [errors, setErrors] = useState<{ sources?: string; results?: string; form?: string }>({});
	const locked = sessions.some(sessionIsActive);
	const projectKey = hostId ? ["project", hostId, projectId] as const : ["project", projectId] as const;

	const save = async (nextSources: string[], nextResults: string) => {
		setBusy(true);
		setErrors({});
		const previousSources = sources;
		const previousResults = resultsDir;
		setSources(nextSources);
		setResultsDir(nextResults);
		try {
			const client = hostId ? clientForHost(hostId) : apiClient;
			const current = await client.GET("/api/v1/projects/{id}", { params: { path: { id: projectId } } });
			if (current.error || current.data?.status !== "ok" || !current.data.project) {
				throw new Error(current.error ? apiErrorMessage(current.error) : t("settings.project.degraded"));
			}
			const latest = current.data.project as Project;
			const { error } = await client.PUT("/api/v1/projects/{id}", {
				params: { path: { id: projectId } },
				body: {
					displayName: latest.name,
					config: { ...latest.config, work: { sources: nextSources, resultsDir: nextResults } },
				},
			});
			if (error) {
				const failure = new Error(apiErrorMessage(error)) as Error & { code?: string };
				failure.code = apiErrorCode(error);
				throw failure;
			}
			await queryClient.invalidateQueries({ queryKey: projectKey });
			await queryClient.invalidateQueries({ queryKey: hostId ? workspaceQueryKeyForHost(hostId) : workspaceQueryKey });
		} catch (error) {
			setSources(previousSources);
			setResultsDir(previousResults);
			const code = error instanceof Error && "code" in error ? String((error as { code?: unknown }).code ?? "") : "";
			const message = error instanceof Error ? error.message : "";
			const field = fieldFor(code);
			if (field === "form" && (code === "WORK_PATH_OVERLAP" || code === "WORK_PATH_UNDER_AO")) {
				setErrors({ sources: message, results: message });
			} else {
				setErrors({ [field]: message });
			}
		} finally {
			setBusy(false);
		}
	};

	const addPaths = (paths: string[]) => {
		const next = [...sources];
		for (const path of paths) {
			if (path && !next.includes(path)) next.push(path);
		}
		if (next.length !== sources.length) void save(next, resultsDir);
	};

	return (
		<div className="flex flex-col gap-(--size-settings-section-gap)">
			<div className="flex flex-col gap-2">
				<span className="text-sm text-settings-label">{t("settings.project.workOriginals")}</span>
				<div className="flex flex-wrap gap-2">
					<Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => {
						void aoBridge.app.chooseDirectory(t("createProject.addFolder")).then((path) => { if (path) addPaths([path]); });
					}}>{t("createProject.addFolder")}</Button>
					<Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => {
						void aoBridge.app.chooseFiles(t("createProject.addFiles")).then((paths) => addPaths(paths));
					}}>{t("createProject.addFiles")}</Button>
				</div>
				{sources.length > 0 ? (
					<ul className="flex flex-col gap-1">
						{sources.map((source) => (
							<li key={source} className="flex min-w-0 items-center gap-2 text-sm">
								<FileText className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
								<span className="min-w-0 flex-1 truncate" title={source}>{baseName(source)}</span>
								<button type="button" className="text-xs text-muted-foreground hover:text-foreground" disabled={busy} aria-label={t("createProject.removeOriginal", { name: baseName(source) })} onClick={() => void save(sources.filter((item) => item !== source), resultsDir)}>
									{t("shell.remove")}
								</button>
							</li>
						))}
					</ul>
				) : <p className="text-xs text-settings-muted">{t("settings.project.workNoOriginals")}</p>}
				{errors.sources ? <p className="text-xs text-error" role="alert">{errors.sources}</p> : null}
			</div>
			<div className="flex flex-col gap-1.5">
				<label className="text-sm text-settings-label" htmlFor="workSettingsResults">{t("settings.project.workResults")}</label>
				<div className="flex min-w-0 items-center gap-2">
					<Input id="workSettingsResults" readOnly value={resultsDir} className="min-w-0 flex-1" />
					<Button type="button" variant="outline" size="sm" disabled={busy || locked} onClick={() => {
						void aoBridge.app.chooseDirectory({ title: t("settings.project.workResults"), defaultPath: resultsDir || undefined }).then((path) => {
							if (path && path !== resultsDir) void save(sources, path);
						});
					}}>{t("createProject.changeResults")}</Button>
				</div>
				{locked ? <p className="text-xs text-settings-muted">{t("settings.project.workResultsLocked")}</p> : null}
				{errors.results ? <p className="text-xs text-error" role="alert">{errors.results}</p> : null}
			</div>
		</div>
	);
}
