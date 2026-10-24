import "reflect-metadata";

import { NestFactory } from "@nestjs/core";
import { FastifyAdapter, type NestFastifyApplication } from "@nestjs/platform-fastify";

import {
  InMemoryIdempotencyRepository,
  InMemoryLabRepository,
  SystemClock,
} from "./adapters/in-memory.js";
import { AppModule } from "./app.module.js";
import type { Clock, IdempotencyRepository, LabRepository } from "./domain/ports.js";
import { PublicationService } from "./domain/publication-service.js";
import { ProblemDetailsFilter } from "./http/problem-details.filter.js";

export interface ApiDependencies {
  readonly authSecret: string;
  readonly clock?: Clock;
  readonly idempotency?: IdempotencyRepository;
  readonly labs?: LabRepository;
  readonly nowSeconds?: () => number;
  readonly webOrigin?: string;
}

export async function createApiApplication(
  dependencies: ApiDependencies,
): Promise<NestFastifyApplication> {
  if (Buffer.byteLength(dependencies.authSecret, "utf8") < 32) {
    throw new Error("AUTH_TOKEN_SECRET must contain at least 32 UTF-8 bytes");
  }

  const publicationService = new PublicationService(
    dependencies.labs ?? new InMemoryLabRepository(),
    dependencies.idempotency ?? new InMemoryIdempotencyRepository(),
    dependencies.clock ?? new SystemClock(),
  );
  const application = await NestFactory.create<NestFastifyApplication>(
    AppModule.register({
      authSecret: dependencies.authSecret,
      nowSeconds: dependencies.nowSeconds ?? (() => Math.floor(Date.now() / 1000)),
      publicationService,
    }),
    new FastifyAdapter({ bodyLimit: 1024 }),
    { logger: false },
  );

  application.enableCors({
    origin: dependencies.webOrigin ?? "http://127.0.0.1:3000",
    methods: ["GET", "POST"],
  });
  application.useGlobalFilters(new ProblemDetailsFilter());
  await application.init();
  await application.getHttpAdapter().getInstance().ready();
  return application;
}
