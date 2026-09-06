import type { ImgHTMLAttributes, ReactElement } from "react";
import brandMarkUrl from "../../assets/brand/studyguardian-mark.svg";

export function BrandMark(props: Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "alt">): ReactElement {
  return <img {...props} src={brandMarkUrl} alt="" aria-hidden="true" />;
}
