import type { CSSProperties } from "react";

const paths = {
  arrow: "M5 12h14m-6-6 6 6-6 6",
  code: "m8 7-5 5 5 5m8-10 5 5-5 5m-3-13-2 16",
  labs: "M5 3h14v18H5zM9 7h6M9 11h6M9 15h3",
  classes: "M3 4h7l2 2h9v14H3z",
  review: "M9 3h6v4H9zM6 5H4v16h16V5h-2M8 14l3 3 5-6",
  search: "M10.5 17a6.5 6.5 0 1 0 0-13 6.5 6.5 0 0 0 0 13Zm5-1 5 5",
  lock: "M5 10h14v11H5zM8 10V6a4 4 0 0 1 8 0v4m-4 4v3",
  clock: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Zm0 4v5l3 2",
  chevron: "m9 5 7 7-7 7",
  check: "m5 12 4 4L19 6",
} as const;

export function Icon({
  name,
  size = 20,
  style,
}: Readonly<{
  name: keyof typeof paths;
  size?: number;
  style?: CSSProperties;
}>) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="square"
      strokeLinejoin="miter"
      aria-hidden="true"
      style={style}
    >
      <path d={paths[name]} />
    </svg>
  );
}
