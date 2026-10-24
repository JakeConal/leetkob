export interface HealthStatus {
  readonly service: "leetkob-api";
  readonly status: "ok";
}

const apiBaseUrl = process.env["NEXT_PUBLIC_API_BASE_URL"] ?? "http://127.0.0.1:3001";

function isHealthStatus(value: unknown): value is HealthStatus {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const record = value as Record<string, unknown>;
  return record["service"] === "leetkob-api" && record["status"] === "ok";
}

export async function fetchHealth(fetcher: typeof fetch = fetch): Promise<HealthStatus> {
  const response = await fetcher(`${apiBaseUrl}/health`, {
    headers: { accept: "application/json" },
  });
  if (!response.ok) throw new Error(`API health request failed with status ${response.status}`);

  const value: unknown = await response.json();
  if (!isHealthStatus(value)) throw new Error("API health response did not match its contract");
  return value;
}
