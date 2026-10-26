import { GitBranch } from "lucide-react";
import { useTruncatedText } from "../../hooks/useTruncatedText";
import { sessionBranchPresentation } from "../../lib/session-branch";
import type { WorkspaceSession } from "../../types/workspace";
import { BranchNamingLabel } from "../BranchNamingLabel";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip";

/**
 * Read-only project and branch line attached to the composer. Branch switching
 * stays out until the daemon can change the checkout.
 */
export function ComposerContextStrip({ session }: { session?: WorkspaceSession }) {
	const projectName = session?.workspaceName.trim() ?? "";
	if (!session || projectName === "") return null;

	const { branch, naming } = sessionBranchPresentation(session);
	const showBranch = Boolean(branch) || naming;

	return (
		<div
			className="flex min-w-0 items-center gap-1.5 px-3 pb-1 text-xs text-muted-foreground"
			data-testid="composer-context-strip"
		>
			<ContextPart value={projectName} />
			{showBranch ? (
				<>
					<span aria-hidden="true" className="shrink-0">·</span>
					<span className="flex min-w-0 items-center gap-1 font-mono">
						<GitBranch aria-hidden="true" className="size-3 shrink-0" />
						{naming ? <BranchNamingLabel /> : <ContextPart value={branch ?? ""} />}
					</span>
				</>
			) : null}
		</div>
	);
}

function ContextPart({ value }: { value: string }) {
	const { ref, isTruncated } = useTruncatedText<HTMLSpanElement>(value);
	const text = (
		<span
			ref={ref}
			className="min-w-0 truncate focus-visible:outline focus-visible:outline-ring"
			data-context-part=""
			tabIndex={isTruncated ? 0 : undefined}
		>
			{value}
		</span>
	);
	if (!isTruncated) return text;
	return (
		<Tooltip>
			<TooltipTrigger asChild>{text}</TooltipTrigger>
			<TooltipContent>{value}</TooltipContent>
		</Tooltip>
	);
}
