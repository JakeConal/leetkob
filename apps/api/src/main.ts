import { createApiApplication } from "./app-factory.js";

function readPort(value: string | undefined): number {
  if (value === undefined || value === "") return 3001;
  const port = Number(value);
  if (!Number.isSafeInteger(port) || port < 1 || port > 65_535) {
    throw new Error("API_PORT must be an integer from 1 through 65535");
  }
  return port;
}

const authSecret = process.env["AUTH_TOKEN_SECRET"];
if (authSecret === undefined) {
  throw new Error("AUTH_TOKEN_SECRET is required");
}

const application = await createApiApplication({
  authSecret,
  webOrigin: process.env["WEB_ORIGIN"] ?? "http://127.0.0.1:3000",
});
await application.listen(readPort(process.env["API_PORT"]), "127.0.0.1");
process.stdout.write(`LeeTKOB API listening on ${await application.getUrl()}\n`);
