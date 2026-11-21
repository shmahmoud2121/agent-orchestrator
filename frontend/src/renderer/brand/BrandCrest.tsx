import { cn } from "../lib/utils";

type BrandCrestProps = {
	/** Pixel size; null leaves width and height to the className. */
	size?: number | null;
	className?: string;
	title?: string;
};

export function BrandCrest({ className, title, size = 16 }: BrandCrestProps) {
	const label = title || undefined;
	return (
		<svg
			aria-hidden={label ? undefined : "true"}
			aria-label={label}
			className={cn("shrink-0", className)}
			height={size ?? undefined}
			role={label ? "img" : undefined}
			viewBox="0 0 48 48"
			width={size ?? undefined}
		>
			<path
				d="M5 35V13l10 12 9-15 9 15 10-12v22"
				fill="none"
				stroke="currentColor"
				strokeWidth={5}
				strokeLinejoin="miter"
				strokeLinecap="square"
			/>
			<path d="M15 34h18" fill="none" stroke="currentColor" strokeWidth={4} />
			<circle cx="24" cy="40" r="3.3" fill="var(--accent-brand)" />
		</svg>
	);
}
