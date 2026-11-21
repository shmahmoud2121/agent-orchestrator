import { BrandCrest } from "../brand";

// The startup mark is the Meiyo crest; the app icon PNG is still Altair until the packaging slice.
export function AOMascot({ className }: { className: string }) {
	return <BrandCrest className={className} size={null} />;
}
