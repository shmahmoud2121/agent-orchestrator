import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { NavRowHighlight } from "./NavRowHighlight";

function highlight(container: HTMLElement) {
	const node = container.querySelector("[data-nav-row-highlight]");
	if (!node) throw new Error("highlight missing");
	return node;
}

describe("NavRowHighlight active marker", () => {
	it("marks an enabled active row", () => {
		const { container } = render(<NavRowHighlight active />);
		expect(highlight(container)).toHaveAttribute("data-nav-row-highlight-active", "");
	});

	it("leaves a disabled row unmarked, including when that row is also active", () => {
		const disabled = render(<NavRowHighlight disabled />);
		expect(highlight(disabled.container)).not.toHaveAttribute("data-nav-row-highlight-active");

		const activeAndDisabled = render(<NavRowHighlight active disabled />);
		expect(highlight(activeAndDisabled.container)).not.toHaveAttribute("data-nav-row-highlight-active");
	});
});
