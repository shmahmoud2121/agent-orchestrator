import { BRAND } from "./brand";
import { BrandCrest } from "./BrandCrest";

/** Meiyo brand mark: the M crest followed by the MEIYO wordmark. */
export function BrandWordmark() {
	return (
		<span className="inline-flex min-w-0 items-center gap-1.5">
			<BrandCrest size={16} />
			<span
				className="min-w-0 truncate"
				data-brand-label=""
				style={{ fontFamily: "var(--font-family-display)", fontWeight: 600, letterSpacing: "3px" }}
			>
				{BRAND.wordmark}
			</span>
		</span>
	);
}
