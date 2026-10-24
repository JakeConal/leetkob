import { createHmac, timingSafeEqual } from "node:crypto";

import type { Identity, Role } from "../domain/types.js";

interface TokenClaims {
  readonly sub: string;
  readonly role: Role;
  readonly classIds: readonly string[];
  readonly exp: number;
}

const roles = new Set<Role>(["admin", "student", "ta"]);
const bearerPattern = /^Bearer ([A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)$/i;

function hasStrongEnoughSecret(secret: string): boolean {
  return Buffer.byteLength(secret, "utf8") >= 32;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isRole(value: unknown): value is Role {
  return typeof value === "string" && roles.has(value as Role);
}

function parseClaims(value: unknown, nowSeconds: number): TokenClaims | undefined {
  if (!isRecord(value)) return undefined;

  const { sub, role, classIds, exp } = value;
  if (typeof sub !== "string" || sub.length < 1 || sub.length > 128) return undefined;
  if (!isRole(role)) return undefined;
  if (!Array.isArray(classIds) || classIds.length > 100) return undefined;
  if (
    !classIds.every(
      (classId) => typeof classId === "string" && classId.length >= 1 && classId.length <= 128,
    )
  ) {
    return undefined;
  }
  if (new Set(classIds).size !== classIds.length) return undefined;
  if (typeof exp !== "number" || !Number.isSafeInteger(exp) || exp <= nowSeconds) return undefined;

  return { sub, role, classIds, exp };
}

function signatureFor(payloadSegment: string, secret: string): Buffer {
  return createHmac("sha256", secret).update(payloadSegment, "ascii").digest();
}

export function issueToken(
  identity: Identity,
  secret: string,
  expiresAtEpochSeconds: number,
): string {
  if (!hasStrongEnoughSecret(secret)) {
    throw new Error("AUTH_TOKEN_SECRET must contain at least 32 UTF-8 bytes");
  }

  const claims = parseClaims(
    {
      sub: identity.subject,
      role: identity.role,
      classIds: [...identity.classIds],
      exp: expiresAtEpochSeconds,
    },
    0,
  );
  if (claims === undefined) throw new Error("Cannot issue a token for invalid claims");

  const payloadSegment = Buffer.from(JSON.stringify(claims), "utf8").toString("base64url");
  const signatureSegment = signatureFor(payloadSegment, secret).toString("base64url");
  return `${payloadSegment}.${signatureSegment}`;
}

export function verifyAuthorizationHeader(
  authorization: string | undefined,
  secret: string,
  nowSeconds = Math.floor(Date.now() / 1000),
): Identity | undefined {
  if (
    authorization === undefined ||
    authorization.length > 8192 ||
    !hasStrongEnoughSecret(secret)
  ) {
    return undefined;
  }

  const match = bearerPattern.exec(authorization);
  if (match === null) return undefined;

  const token = match[1];
  if (token === undefined) return undefined;
  const separator = token.indexOf(".");
  if (separator < 1) return undefined;

  const payloadSegment = token.slice(0, separator);
  const signatureSegment = token.slice(separator + 1);

  try {
    const supplied = Buffer.from(signatureSegment, "base64url");
    const expected = signatureFor(payloadSegment, secret);
    if (supplied.length !== expected.length || !timingSafeEqual(supplied, expected))
      return undefined;

    const decoded: unknown = JSON.parse(Buffer.from(payloadSegment, "base64url").toString("utf8"));
    const claims = parseClaims(decoded, nowSeconds);
    if (claims === undefined) return undefined;

    return {
      subject: claims.sub,
      role: claims.role,
      classIds: [...claims.classIds],
    };
  } catch {
    return undefined;
  }
}
