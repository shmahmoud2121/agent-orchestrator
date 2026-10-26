import { ChevronRight, X } from "lucide-react";
import { useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { BUILTIN_WORKER_PROTECTED_PATHS } from "../../lib/worker-protected-paths";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "../ui/accordion";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip";
import { SettingsRow } from "./SettingsRow";

/** Worker-sandbox "Protected files": the built-in list (read-only) and the project's own additions. */
export function ProtectedFilesSettings({ paths, onChange }: { paths: string[]; onChange: (paths: string[]) => void }) {
	const { t } = useTranslation();
	const [draft, setDraft] = useState("");
	const entry = draft.trim();
	const add = () => {
		if (!entry || paths.includes(entry)) return;
		onChange([...paths, entry]);
		setDraft("");
	};
	const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
		if (event.key !== "Enter") return;
		// The settings page is a form; Enter adds the entry instead of submitting it.
		event.preventDefault();
		add();
	};
	return <>
		<SettingsRow label={t("settings.project.protectedFiles")} description={t("settings.project.protectedFilesDescription")}>
			<div className="flex items-center gap-2">
				<Input
					id="project-protected-file-new"
					aria-label={t("settings.project.protectedFilesNew")}
					className="w-56 max-w-full font-mono"
					autoComplete="off"
					spellCheck={false}
					value={draft}
					placeholder="config/production.yml"
					onChange={(event) => setDraft(event.target.value)}
					onKeyDown={onKeyDown}
				/>
				<Button type="button" variant="secondary" disabled={!entry || paths.includes(entry)} onClick={add}>{t("settings.project.protectedFilesAdd")}</Button>
			</div>
		</SettingsRow>
		{paths.map((path) => {
			const label = t("settings.project.protectedFilesRemove", { path });
			return <SettingsRow key={path} label={<span className="font-mono">{path}</span>}>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button type="button" variant="ghost" size="icon-sm" aria-label={label} onClick={() => onChange(paths.filter((item) => item !== path))}>
							<X className="size-icon-md" aria-hidden="true" />
						</Button>
					</TooltipTrigger>
					<TooltipContent>{label}</TooltipContent>
				</Tooltip>
			</SettingsRow>;
		})}
		<Accordion type="single" collapsible>
			<AccordionItem value="builtin" className="border-0">
				<AccordionTrigger
					className="settings-row-bar gap-1 text-sm text-settings-label"
					headerClassName="rounded-none hover:bg-transparent data-[state=open]:bg-transparent"
				>
					<span className="inline-flex items-center gap-1">
						{t("settings.project.protectedFilesBuiltin")}
						<ChevronRight aria-hidden="true" className="size-icon-md shrink-0 text-settings-muted transition-transform duration-150 group-data-[state=open]/row:rotate-90" />
					</span>
				</AccordionTrigger>
				<AccordionContent>
					<dl className="space-y-2 px-(--settings-row-inset) pb-2.5 text-control leading-4">
						<BuiltinGroup label={t("settings.project.protectedFilesBuiltinProject")} entries={BUILTIN_WORKER_PROTECTED_PATHS.mainCheckout} />
						<BuiltinGroup label={t("settings.project.protectedFilesBuiltinWorktree")} entries={BUILTIN_WORKER_PROTECTED_PATHS.worktree} />
						<BuiltinGroup label={t("settings.project.protectedFilesBuiltinHome")} entries={BUILTIN_WORKER_PROTECTED_PATHS.home.map((path) => `~/${path}`)} />
						<BuiltinGroup label={t("settings.project.protectedFilesBuiltinToken")} entries={["api-token"]} />
						<p className="text-settings-muted">{t("settings.project.protectedFilesSamples")}</p>
					</dl>
				</AccordionContent>
			</AccordionItem>
		</Accordion>
	</>;
}

function BuiltinGroup({ label, entries }: { label: string; entries: readonly string[] }) {
	return <div>
		<dt className="text-settings-label">{label}</dt>
		<dd className="mt-0.5 font-mono text-settings-muted">{entries.join("  ")}</dd>
	</div>;
}
