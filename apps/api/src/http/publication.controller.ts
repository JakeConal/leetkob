import { Controller, Get, Inject, Param, Post, Req, Res } from "@nestjs/common";
import type { FastifyReply, FastifyRequest } from "fastify";

import { AUTH_SECRET, NOW_SECONDS, PUBLICATION_SERVICE } from "../application.tokens.js";
import { verifyAuthorizationHeader } from "../auth/token.js";
import { problemResponse } from "../domain/problems.js";
import type { PublicationService } from "../domain/publication-service.js";
import { sendStoredResponse } from "./send-response.js";

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const idempotencyKeyPattern = /^[\x21-\x7e]{8,128}$/;

function headerValue(value: string | string[] | undefined): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function pathname(request: FastifyRequest): string {
  return request.url.split("?", 1)[0] ?? "/";
}

@Controller()
export class HealthController {
  @Get("health")
  health(@Res() reply: FastifyReply): void {
    void reply.header("cache-control", "no-store").send({ status: "ok", service: "leetkob-api" });
  }
}

@Controller("labs")
export class PublicationController {
  constructor(
    @Inject(PUBLICATION_SERVICE) private readonly publicationService: PublicationService,
    @Inject(AUTH_SECRET) private readonly authSecret: string,
    @Inject(NOW_SECONDS) private readonly nowSeconds: () => number,
  ) {}

  @Post(":labId/publish")
  async publish(
    @Param("labId") labId: string,
    @Req() request: FastifyRequest,
    @Res() reply: FastifyReply,
  ): Promise<void> {
    const instance = pathname(request);
    if (request.url.includes("?")) {
      sendStoredResponse(
        reply,
        problemResponse(
          "route-not-found",
          404,
          "Route not found",
          "No route matches this request.",
          instance,
        ),
      );
      return;
    }

    const identity = verifyAuthorizationHeader(
      headerValue(request.headers.authorization),
      this.authSecret,
      this.nowSeconds(),
    );
    if (identity === undefined) {
      sendStoredResponse(
        reply,
        problemResponse(
          "authentication-required",
          401,
          "Authentication required",
          "Provide a valid, unexpired bearer token.",
          instance,
        ),
      );
      return;
    }

    const idempotencyKey = headerValue(request.headers["idempotency-key"]);
    if (
      !uuidPattern.test(labId) ||
      idempotencyKey === undefined ||
      !idempotencyKeyPattern.test(idempotencyKey) ||
      request.body !== undefined
    ) {
      sendStoredResponse(
        reply,
        problemResponse(
          "invalid-request",
          400,
          "Invalid request",
          "Use a UUID labId, an 8-128 character visible-ASCII Idempotency-Key, and an empty body.",
          instance,
        ),
      );
      return;
    }

    const response = await this.publicationService.publish({
      identity,
      labId,
      idempotencyKey,
      instance,
    });
    sendStoredResponse(reply, response);
  }
}
