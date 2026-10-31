import altairMark from "../assets/brand/altair-mark.png";

// The app's brand mark, shown on the startup screen. It is the same "A" image as
// the app icon (assets/icon.png), so the loader never shows a different logo.
// The old star mark is kept at assets/retired/star/mascot.svg.
export function AOMascot({ className }: { className: string }) {
	return <img className={`inline-block rounded-[22%] ${className}`} src={altairMark} alt="" aria-hidden="true" draggable={false} />;
}
