import type { LabSummary } from "./live/api";

export type LabAvailability = "Open" | "Upcoming" | "Closed";

export function labAvailability(
  lab: Pick<LabSummary, "opensAt" | "deadline">,
  now: number,
): LabAvailability {
  if (now < Date.parse(lab.opensAt)) return "Upcoming";
  return now < Date.parse(lab.deadline) ? "Open" : "Closed";
}

export function filterLabs(
  labs: readonly LabSummary[],
  query: string,
  availability: string,
  now: number,
): readonly LabSummary[] {
  const search = query.trim().toLocaleLowerCase();
  return labs.filter(
    (lab) =>
      (availability === "All labs" || labAvailability(lab, now) === availability) &&
      (lab.title.toLocaleLowerCase().includes(search) ||
        lab.id.toLocaleLowerCase().includes(search)),
  );
}
