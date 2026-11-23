export const liveApiPathTemplates = {
  login: "/v2/auth/login",
  me: "/v2/auth/me",
  logout: "/v2/auth/logout",
  classes: "/v2/classes",
  classLabs: "/v2/classes/{classId}/labs",
  lab: "/v2/labs/{labId}",
  submissions: "/v2/labs/{labId}/submissions",
  submission: "/v2/submissions/{submissionId}",
  reviews: "/v2/labs/{labId}/reviews",
  decision: "/v2/labs/{labId}/reviews/{findingId}/decision",
} as const;

export function fillLivePath(template: string, values: Readonly<Record<string, string>>): string {
  return template.replace(/\{([^}]+)\}/g, (_match, key: string) => {
    const value = values[key];
    if (value === undefined) throw new Error(`Missing path value ${key}`);
    return encodeURIComponent(value);
  });
}

export const liveApi = {
  login: liveApiPathTemplates.login,
  me: liveApiPathTemplates.me,
  logout: liveApiPathTemplates.logout,
  classes: liveApiPathTemplates.classes,
  classLabs: (classId: string) => fillLivePath(liveApiPathTemplates.classLabs, { classId }),
  lab: (labId: string) => fillLivePath(liveApiPathTemplates.lab, { labId }),
  submissions: (labId: string) => fillLivePath(liveApiPathTemplates.submissions, { labId }),
  submission: (submissionId: string) =>
    fillLivePath(liveApiPathTemplates.submission, { submissionId }),
  reviews: (labId: string) => fillLivePath(liveApiPathTemplates.reviews, { labId }),
  decision: (labId: string, findingId: string) =>
    fillLivePath(liveApiPathTemplates.decision, { labId, findingId }),
} as const;

export function withCursor(path: string, cursor: string | null): string {
  if (cursor === null) return path;
  return `${path}?${new URLSearchParams({ cursor }).toString()}`;
}
