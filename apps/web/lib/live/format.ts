const dateTime = new Intl.DateTimeFormat("en-GB", {
  day: "2-digit",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  timeZone: "UTC",
  timeZoneName: "short",
});

/** Formats an ISO timestamp for display; returns the raw value when it does not parse. */
export function formatDateTime(iso: string): string {
  const time = Date.parse(iso);
  return Number.isNaN(time) ? iso : dateTime.format(time);
}
