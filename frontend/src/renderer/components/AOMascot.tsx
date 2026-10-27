export function AOMascot({ className }: { className: string }) {
	return (
		<svg className={`inline-block ${className}`} viewBox="0 0 1024 1024" aria-hidden="true">
			<path fill="var(--accent-star)" d="M512 195L607 382L744 512L607 642L512 829L417 642L280 512L417 382Z" />
		</svg>
	);
}
