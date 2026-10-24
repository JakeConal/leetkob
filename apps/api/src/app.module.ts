import { Module, type DynamicModule } from "@nestjs/common";

import { AUTH_SECRET, NOW_SECONDS, PUBLICATION_SERVICE } from "./application.tokens.js";
import type { PublicationService } from "./domain/publication-service.js";
import { HealthController, PublicationController } from "./http/publication.controller.js";

export interface ApplicationBindings {
  readonly authSecret: string;
  readonly nowSeconds: () => number;
  readonly publicationService: PublicationService;
}

@Module({})
export class AppModule {
  static register(bindings: ApplicationBindings): DynamicModule {
    return {
      module: AppModule,
      controllers: [HealthController, PublicationController],
      providers: [
        { provide: AUTH_SECRET, useValue: bindings.authSecret },
        { provide: NOW_SECONDS, useValue: bindings.nowSeconds },
        { provide: PUBLICATION_SERVICE, useValue: bindings.publicationService },
      ],
    };
  }
}
