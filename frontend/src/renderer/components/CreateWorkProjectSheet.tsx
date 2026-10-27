import * as Dialog from "@radix-ui/react-dialog";
import { FileText, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAgentReadinessQuery, useEnsureAgentReadiness } from "../hooks/useAgentReadinessQuery";
import { aoBridge } from "../lib/bridge";
import { cn } from "../lib/utils";
import { RequiredAgentField } from "./CreateProjectAgentSheet";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";

export type CreateWorkProjectInput = {
	path: string;
	name: string;
	sources: string[];
	resultsDir: string;
	workerAgent: string;
	orchestratorAgent: string;
};

type FieldErrors = {
	name?: string;
	sources?: string;
	results?: string;
	form?: string;
};

function joinResults(documents: string, name: string): string {
	const safe = name.trim().replace(/[<>:"/\\|?*\u0000-\u001f]/g, " ").replace(/[. ]+$/g, "").trim() || "project";
	const sep = documents.includes("\\") ? "\\" : "/";
	return `${documents.replace(/[\\/]+$/, "")}${sep}Altair${sep}${safe}`;
}

function fieldErrors(error: unknown): FieldErrors {
	const code = error instanceof Error && "code" in error ? String((error as { code?: unknown }).code ?? "") : "";
	const message = error instanceof Error ? error.message : "";
	if (code === "DISPLAY_NAME_TOO_LONG") return { name: message };
	if (code === "WORK_SOURCE_REQUIRED" || code === "WORK_SOURCE_MISSING" || code === "WORK_SOURCE_NOT_ABSOLUTE" || code === "WORK_SOURCE_TOO_MANY") {
		return { sources: message };
	}
	if (code === "WORK_RESULTS_NOT_ABSOLUTE" || code === "WORK_RESULTS_CREATE" || code === "WORK_RESULTS_LOCKED") {
		return { results: message };
	}
	if (code === "WORK_PATH_OVERLAP" || code === "WORK_PATH_UNDER_AO") {
		return { sources: message, results: message };
	}
	return { form: message || undefined };
}

function baseName(path: string): string {
	const parts = path.split(/[\\/]/).filter(Boolean);
	return parts[parts.length - 1] ?? path;
}

export function CreateWorkProjectSheet({
	open,
	isCreating,
	onOpenChange,
	onSubmit,
}: {
	open: boolean;
	isCreating: boolean;
	onOpenChange: (open: boolean) => void;
	onSubmit: (input: CreateWorkProjectInput) => Promise<void>;
}) {
	const { t } = useTranslation();
	const [name, setName] = useState("");
	const [sources, setSources] = useState<string[]>([]);
	const [documents, setDocuments] = useState("");
	const [customResults, setCustomResults] = useState<string | null>(null);
	const [manager, setManager] = useState("");
	const [worker, setWorker] = useState("");
	const [errors, setErrors] = useState<FieldErrors>({});
	const agentsQuery = useAgentReadinessQuery(open);
	const agents = useMemo(() => agentsQuery.data?.agents ?? [], [agentsQuery.data?.agents]);
	useEnsureAgentReadiness({ agentIds: [manager, worker].filter(Boolean), enabled: open && (manager !== "" || worker !== "") });
	const resultsDir = customResults ?? (documents ? joinResults(documents, name) : "");
	const canCreate = name.trim() !== "" && sources.length > 0 && manager !== "" && worker !== "" && resultsDir !== "" && !isCreating;

	useEffect(() => {
		if (!open) return;
		setName("");
		setSources([]);
		setCustomResults(null);
		setManager("");
		setWorker("");
		setErrors({});
		let cancelled = false;
		void aoBridge.app.documentsPath().then((dir) => {
			if (!cancelled && dir) setDocuments(dir);
		}).catch(() => undefined);
		return () => {
			cancelled = true;
		};
	}, [open]);

	const addPaths = (paths: string[]) => {
		setSources((current) => {
			const next = [...current];
			for (const path of paths) {
				if (path && !next.includes(path)) next.push(path);
			}
			return next;
		});
		setErrors((current) => ({ ...current, sources: undefined }));
	};

	const submit = async () => {
		if (!canCreate) return;
		setErrors({});
		try {
			await onSubmit({
				path: resultsDir,
				name: name.trim(),
				sources,
				resultsDir,
				workerAgent: worker,
				orchestratorAgent: manager,
			});
		} catch (error) {
			setErrors(fieldErrors(error));
		}
	};

	return (
		<Dialog.Root open={open} onOpenChange={(next) => { if (!isCreating) onOpenChange(next); }}>
			<Dialog.Portal>
				<Dialog.Content className="fixed left-1/2 top-1/2 z-overlay w-dialog-lg max-h-[85vh] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-lg border border-border bg-popover p-0 text-popover-foreground shadow-xl">
					<div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
						<Dialog.Title className="text-sm font-medium">{t("createProject.workTitle")}</Dialog.Title>
						<Dialog.Close asChild>
							<button type="button" className="settings-close-button" aria-label={t("createProject.closeDialog")} disabled={isCreating}>
								<X className="size-4" aria-hidden="true" />
							</button>
						</Dialog.Close>
					</div>
					<Dialog.Description className="sr-only">{t("createProject.workProjectDesc")}</Dialog.Description>
					<form className="flex flex-col gap-4 px-4 py-4" onSubmit={(event) => { event.preventDefault(); void submit(); }}>
						<div className="flex flex-col gap-1.5">
							<Label htmlFor="workProjectName">{t("createProject.workName")}</Label>
							<Input id="workProjectName" value={name} onChange={(event) => { setName(event.target.value); setErrors((current) => ({ ...current, name: undefined })); }} autoComplete="off" />
							{errors.name ? <p className="text-xs text-error" role="alert">{errors.name}</p> : null}
						</div>
						<div className="flex flex-col gap-1.5">
							<span className="text-xs font-medium text-muted-foreground">{t("createProject.originals")}</span>
							<div className="flex flex-wrap gap-2">
								<Button type="button" variant="outline" size="sm" onClick={() => {
									void aoBridge.app.chooseDirectory(t("createProject.addFolder")).then((path) => { if (path) addPaths([path]); });
								}}>{t("createProject.addFolder")}</Button>
								<Button type="button" variant="outline" size="sm" onClick={() => {
									void aoBridge.app.chooseFiles(t("createProject.addFiles")).then((paths) => addPaths(paths));
								}}>{t("createProject.addFiles")}</Button>
							</div>
							{sources.length > 0 ? (
								<ul className="flex flex-col gap-1">
									{sources.map((source) => (
										<li key={source} className="flex min-w-0 items-center gap-2 text-sm">
											<FileText className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
											<span className="min-w-0 flex-1 truncate" title={source}>{baseName(source)}</span>
											<button type="button" className="text-xs text-muted-foreground hover:text-foreground" aria-label={t("createProject.removeOriginal", { name: baseName(source) })} onClick={() => setSources((current) => current.filter((item) => item !== source))}>
												{t("shell.remove")}
											</button>
										</li>
									))}
								</ul>
							) : null}
							{errors.sources ? <p className="text-xs text-error" role="alert">{errors.sources}</p> : null}
						</div>
						<div className="flex flex-col gap-1.5">
							<Label htmlFor="workResultsDir">{t("createProject.resultsFolder")}</Label>
							<div className="flex min-w-0 items-center gap-2">
								<Input id="workResultsDir" readOnly value={resultsDir} className="min-w-0 flex-1" />
								<Button type="button" variant="outline" size="sm" onClick={() => {
									void aoBridge.app.chooseDirectory({ title: t("createProject.resultsFolder"), defaultPath: resultsDir || undefined }).then((path) => {
										if (path) {
											setCustomResults(path);
											setErrors((current) => ({ ...current, results: undefined }));
										}
									});
								}}>{t("createProject.changeResults")}</Button>
							</div>
							{errors.results ? <p className="text-xs text-error" role="alert">{errors.results}</p> : null}
						</div>
						<RequiredAgentField
							id="workManager"
							label={t("createProject.workManager")}
							hint={t("createProject.chooseManager")}
							placeholder={t("createProject.selectManager")}
							value={manager}
							agents={agents}
							watchedOnly
							onChange={setManager}
						/>
						<RequiredAgentField
							id="workWorker"
							label={t("createProject.workWorkers")}
							hint={t("createProject.chooseWorker")}
							placeholder={t("createProject.selectWorkers")}
							value={worker}
							agents={agents}
							watchedOnly
							onChange={setWorker}
						/>
						{errors.form ? <p className="text-xs text-error" role="alert">{errors.form}</p> : null}
						<div className="flex justify-end">
							<Button type="submit" disabled={!canCreate} className={cn(!canCreate && "opacity-50")}>{t("createProject.create")}</Button>
						</div>
					</form>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	);
}
