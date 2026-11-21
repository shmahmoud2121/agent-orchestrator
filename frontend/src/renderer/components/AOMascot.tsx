import { BrandCrest } from "../brand";

// The startup mark is the Meiyo crest, the same mark as the app icon (assets/meiyo-icon-source.svg).
export function AOMascot({ className }: { className: string }) {
	return <BrandCrest className={className} size={null} />;
}
