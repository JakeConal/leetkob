import type { FastifyReply } from "fastify";

import type { StoredResponse } from "../domain/types.js";

export function sendStoredResponse(reply: FastifyReply, response: StoredResponse): void {
  void reply
    .code(response.status)
    .header("cache-control", "no-store")
    .type(response.contentType)
    .send(response.body);
}
