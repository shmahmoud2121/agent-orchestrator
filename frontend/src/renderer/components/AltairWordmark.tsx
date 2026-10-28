/** Altair brand mark: gold four-point star followed by the ALTAIR wordmark. */
export function AltairWordmark() {
	return (
		<span className="inline-flex min-w-0 items-center gap-1.5">
			<svg
				aria-hidden="true"
				className="shrink-0"
				height="16"
				viewBox="0 0 64 64"
				width="16"
			>
				<path d="M32 6 L37 27 L58 32 L37 37 L32 58 L27 37 L6 32 L27 27 Z" fill="var(--accent-star)" />
			</svg>
			<span
				className="min-w-0 truncate"
				data-brand-label=""
				style={{ fontFamily: "var(--font-family-display)", fontWeight: 600, letterSpacing: "3px" }}
			>
				ALTAIR
			</span>
		</span>
	);
}
