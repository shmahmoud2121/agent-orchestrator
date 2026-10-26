import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceSession } from "../../types/workspace";
import { TooltipProvider } from "../ui/tooltip";
import { ComposerContextStrip } from "./ComposerContextStrip";

const namingLabels = [
	"Generating branch name",
	"Naming branch",
	"Preparing branch",
	"Initializing branch",
	"Resolving branch name",
];

function session(overrides: Partial<WorkspaceSession> = {}): WorkspaceSession {
	return {
		id: "s1",
		workspaceId: "project-1",
		workspaceName: "agent-orchestrator",
		title: "Review the composer",
		provider: "codex",
		kind: "worker",
		mode: "chat",
		status: "working",
		branch: "ao/fix-login/root",
		updatedAt: "2026-08-15T00:00:00Z",
		prs: [],
		...overrides,
	};
}

function renderStrip(ui: ReactElement) {
	return render(<TooltipProvider delayDuration={0}>{ui}</TooltipProvider>);
}

describe("ComposerContextStrip", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("renders the project name and the branch", () => {
		renderStrip(<ComposerContextStrip session={session()} />);

		const strip = screen.getByTestId("composer-context-strip");
		expect(strip).toHaveTextContent("agent-orchestrator");
		expect(strip).toHaveTextContent("·");
		const branch = screen.getByText("ao/fix-login/root");
		expect(branch.closest(".font-mono")).not.toBeNull();
		expect(screen.getByText("agent-orchestrator").closest(".font-mono")).toBeNull();
		expect(strip.querySelector("svg")).toHaveClass("size-3");
	});

	it("hides the branch and shows the naming label while the worktree is still being named", () => {
		renderStrip(
			<ComposerContextStrip
				session={session({
					branch: "ao/provisional",
					provisionState: "provisioning",
					provisionSteps: [{ id: "worktree", status: "running" }],
				})}
			/>,
		);

		expect(screen.queryByText("ao/provisional")).not.toBeInTheDocument();
		expect(screen.getByRole("status")).toHaveTextContent(new RegExp(namingLabels.join("|")));
		expect(screen.getByText("agent-orchestrator")).toBeInTheDocument();
	});

	it("renders nothing without a session or a project name", () => {
		const { container, rerender } = renderStrip(<ComposerContextStrip />);
		expect(screen.queryByTestId("composer-context-strip")).not.toBeInTheDocument();

		rerender(<TooltipProvider delayDuration={0}><ComposerContextStrip session={session({ workspaceName: "  " })} /></TooltipProvider>);
		expect(container.querySelector("[data-testid='composer-context-strip']")).toBeNull();
	});

	it("truncates long values and shows each full value in a tooltip", async () => {
		const user = userEvent.setup();
		vi.spyOn(Element.prototype, "scrollWidth", "get").mockImplementation(function (this: Element) {
			return this.hasAttribute("data-context-part") ? 320 : 0;
		});
		vi.spyOn(Element.prototype, "clientWidth", "get").mockImplementation(function (this: Element) {
			return this.hasAttribute("data-context-part") ? 48 : 0;
		});
		const projectName = "a-very-long-project-name-that-does-not-fit-in-the-strip";
		const branch = "ao/an-equally-long-branch-name/root";
		renderStrip(<ComposerContextStrip session={session({ workspaceName: projectName, branch })} />);

		const project = screen.getByText(projectName);
		const branchText = screen.getByText(branch);
		expect(project).toHaveClass("truncate");
		expect(branchText).toHaveClass("truncate");

		await user.hover(project);
		expect(await screen.findByRole("tooltip")).toHaveTextContent(projectName);

		await user.unhover(project);
		await user.hover(branchText);
		expect(await screen.findByRole("tooltip")).toHaveTextContent(branch);
	});
});
