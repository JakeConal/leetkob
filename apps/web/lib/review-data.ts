export interface ReviewRow {
  readonly id: number;
  readonly status: "Changes requested" | "In review";
  readonly title: string;
  readonly updated: string;
}

export const currentLab = {
  name: "PA#2 Quality Gate",
  deadlinePassed: true,
  pendingReviews: 3,
} as const;

export function isPublicationBlocked(lab: {
  readonly deadlinePassed: boolean;
  readonly pendingReviews: number;
}): boolean {
  return !lab.deadlinePassed || lab.pendingReviews > 0;
}

export const reviewRows: readonly ReviewRow[] = [
  {
    id: 1,
    title: "API contract matches the specification",
    status: "Changes requested",
    updated: "Today, 10:14",
  },
  {
    id: 2,
    title: "Authentication and authorization boundary",
    status: "In review",
    updated: "Today, 09:31",
  },
  {
    id: 3,
    title: "Problem details and edge cases",
    status: "In review",
    updated: "Today, 09:12",
  },
];
