export type Role = "admin" | "student" | "ta";

export interface Identity {
  readonly subject: string;
  readonly role: Role;
  readonly classIds: readonly string[];
}

export interface ReadyLab {
  readonly id: string;
  readonly classId: string;
  readonly status: "ready";
  readonly pendingReviews: number;
  readonly submissionDeadline: string;
}

export interface PublishedLab {
  readonly id: string;
  readonly classId: string;
  readonly status: "published";
  readonly pendingReviews: 0;
  readonly submissionDeadline: string;
  readonly publishedAt: string;
  readonly publishedBy: string;
  readonly publicationKey: string;
}

export type Lab = ReadyLab | PublishedLab;

export interface PublicationRepresentation {
  readonly labId: string;
  readonly status: "published";
  readonly publishedAt: string;
  readonly publishedBy: string;
}

export interface ProblemDetails {
  readonly type: string;
  readonly title: string;
  readonly status: number;
  readonly detail: string;
  readonly instance: string;
  readonly [extension: string]: unknown;
}

export interface SuccessResponse {
  readonly status: 200;
  readonly contentType: "application/json";
  readonly body: PublicationRepresentation;
}

export interface ProblemResponse {
  readonly status: number;
  readonly contentType: "application/problem+json";
  readonly body: ProblemDetails;
}

export type StoredResponse = ProblemResponse | SuccessResponse;

export interface PublishInput {
  readonly identity: Identity;
  readonly labId: string;
  readonly idempotencyKey: string;
  readonly instance: string;
}
