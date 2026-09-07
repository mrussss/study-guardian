import type { ImgHTMLAttributes, ReactElement } from "react";

const brandMarkUrl = new URL("../../assets/brand/studyguardian-mark.svg", import.meta.url).href;

export function BrandMark(props: Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "alt">): ReactElement {
  return <img {...props} src={brandMarkUrl} alt="" aria-hidden="true" />;
}
