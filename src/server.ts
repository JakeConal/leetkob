import { createServer } from "node:http";

import {
  InMemoryIdempotencyRepository,
  InMemoryLabRepository,
  SystemClock,
} from "./adapters/in-memory.js";
import { PublicationService } from "./domain/publication-service.js";
import { createRequestHandler } from "./http/app.js";

function readPort(value: string | undefined): number {
  if (value === undefined || value === "") return 3000;
  const port = Number(value);
  if (!Number.isSafeInteger(port) || port < 1 || port > 65_535) {
    throw new Error("PORT must be an integer from 1 through 65535");
  }
  return port;
}

const authSecret = process.env["AUTH_TOKEN_SECRET"];
if (authSecret === undefined || Buffer.byteLength(authSecret, "utf8") < 32) {
  throw new Error("AUTH_TOKEN_SECRET must contain at least 32 UTF-8 bytes");
}

const publicationService = new PublicationService(
  new InMemoryLabRepository(),
  new InMemoryIdempotencyRepository(),
  new SystemClock(),
);
const server = createServer(createRequestHandler({ publicationService, authSecret }));
const port = readPort(process.env["PORT"]);

server.listen(port, "127.0.0.1", () => {
  process.stdout.write(`LeeTKOB listening on http://127.0.0.1:${port}\n`);
});
