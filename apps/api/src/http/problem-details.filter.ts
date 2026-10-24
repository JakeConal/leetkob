import { Catch, HttpException, type ArgumentsHost, type ExceptionFilter } from "@nestjs/common";
import type { FastifyReply, FastifyRequest } from "fastify";

import { problemResponse, type ProblemCode } from "../domain/problems.js";
import { sendStoredResponse } from "./send-response.js";

interface ProblemDefinition {
  readonly code: ProblemCode;
  readonly detail: string;
  readonly status: number;
  readonly title: string;
}

function definitionFor(exception: unknown): ProblemDefinition {
  const receivedStatus = exception instanceof HttpException ? exception.getStatus() : 500;
  if (receivedStatus === 404) {
    return {
      code: "route-not-found",
      detail: "No route matches this request.",
      status: 404,
      title: "Route not found",
    };
  }
  if (receivedStatus === 400 || receivedStatus === 413 || receivedStatus === 415) {
    return {
      code: "invalid-request",
      detail: "The request does not match the documented contract.",
      status: 400,
      title: "Invalid request",
    };
  }
  return {
    code: "internal-error",
    detail: "The request could not be completed.",
    status: 500,
    title: "Internal server error",
  };
}

@Catch()
export class ProblemDetailsFilter implements ExceptionFilter {
  catch(exception: unknown, host: ArgumentsHost): void {
    const request = host.switchToHttp().getRequest<FastifyRequest>();
    const reply = host.switchToHttp().getResponse<FastifyReply>();
    const definition = definitionFor(exception);
    const instance = request.url.split("?", 1)[0] ?? "/";

    sendStoredResponse(
      reply,
      problemResponse(
        definition.code,
        definition.status,
        definition.title,
        definition.detail,
        instance,
      ),
    );
  }
}
